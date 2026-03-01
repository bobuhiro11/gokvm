// Package migration provides types and utilities for live migration of gokvm VMs.
// This file implements the framed binary transport used to stream migration data
// between the source and destination over a TCP connection.
//
// Wire format for each message:
//
//	[4-byte big-endian type][8-byte big-endian payload length][payload bytes]
package migration

import (
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
)

// MsgType identifies a migration protocol message.
type MsgType uint32

const (
	MsgSnapshot    MsgType = 1 // gob-encoded Snapshot (no memory)
	MsgMemoryFull  MsgType = 2 // raw guest memory (full copy)
	MsgMemoryDirty MsgType = 3 // raw dirty pages preceded by their bitmap
	MsgDone        MsgType = 4 // source signals end-of-migration
	MsgReady       MsgType = 5 // destination confirms it is running
)

// Sender writes framed messages to an underlying writer (typically a TCP conn).
type Sender struct {
	w io.Writer
}

// NewSender wraps w as a migration Sender.
func NewSender(w io.Writer) *Sender { return &Sender{w: w} }

// send writes a single framed message.
func (s *Sender) send(t MsgType, payload []byte) error {
	hdr := make([]byte, 12)
	binary.BigEndian.PutUint32(hdr[0:4], uint32(t))
	binary.BigEndian.PutUint64(hdr[4:12], uint64(len(payload)))

	if _, err := s.w.Write(hdr); err != nil {
		return fmt.Errorf("send header: %w", err)
	}

	if len(payload) > 0 {
		if _, err := s.w.Write(payload); err != nil {
			return fmt.Errorf("send payload: %w", err)
		}
	}

	return nil
}

// SendSnapshot encodes snap with gob and sends it as a MsgSnapshot.
func (s *Sender) SendSnapshot(snap *Snapshot) error {
	pr, pw := io.Pipe()

	errCh := make(chan error, 1)

	go func() {
		enc := gob.NewEncoder(pw)
		errCh <- enc.Encode(snap)

		pw.Close()
	}()

	payload, err := io.ReadAll(pr)
	if err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}

	if err := <-errCh; err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}

	return s.send(MsgSnapshot, payload)
}

// SendMemoryFull sends the raw memory bytes (full copy).
func (s *Sender) SendMemoryFull(mem []byte) error {
	return s.send(MsgMemoryFull, mem)
}

// SendMemoryDirty sends a dirty-page transfer message.
// bitmap is the raw bitmap ([]uint64 as little-endian bytes) followed by
// the dirty page data; the receiver uses the same bitmap to apply pages.
func (s *Sender) SendMemoryDirty(bitmapBytes []byte, pageData []byte) error {
	// Message layout: [8-byte bitmap length][bitmap][page data]
	hdr := make([]byte, 8)
	binary.BigEndian.PutUint64(hdr, uint64(len(bitmapBytes)))
	payload := make([]byte, 0, 8+len(bitmapBytes)+len(pageData))
	payload = append(payload, hdr...)
	payload = append(payload, bitmapBytes...)
	payload = append(payload, pageData...)

	return s.send(MsgMemoryDirty, payload)
}

// SendDone signals the end of the migration stream.
func (s *Sender) SendDone() error { return s.send(MsgDone, nil) }

// SendReady signals that the destination VM is running.
func (s *Sender) SendReady() error { return s.send(MsgReady, nil) }

// Receiver reads framed messages from an underlying reader.
type Receiver struct {
	r io.Reader
}

// NewReceiver wraps r as a migration Receiver.
func NewReceiver(r io.Reader) *Receiver { return &Receiver{r: r} }

// Next reads the next message header and returns the type and full payload.
func (r *Receiver) Next() (MsgType, []byte, error) {
	hdr := make([]byte, 12)
	if _, err := io.ReadFull(r.r, hdr); err != nil {
		return 0, nil, fmt.Errorf("read header: %w", err)
	}

	t := MsgType(binary.BigEndian.Uint32(hdr[0:4]))
	length := binary.BigEndian.Uint64(hdr[4:12])

	if length == 0 {
		return t, nil, nil
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r.r, payload); err != nil {
		return 0, nil, fmt.Errorf("read payload (type=%d len=%d): %w", t, length, err)
	}

	return t, payload, nil
}

// DecodeSnapshot decodes a gob-encoded Snapshot from payload bytes.
func DecodeSnapshot(payload []byte) (*Snapshot, error) {
	snap := &Snapshot{}
	dec := gob.NewDecoder((*bReader)(&payload))

	if err := dec.Decode(snap); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}

	return snap, nil
}

// sentinel errors for DecodeDirtyPayload.
var (
	errDirtyPayloadTooShort  = errors.New("dirty payload too short")
	errDirtyPayloadTruncated = errors.New("dirty payload truncated")
)

// DecodeDirtyPayload splits a MsgMemoryDirty payload into the bitmap bytes
// and the packed page data bytes.
func DecodeDirtyPayload(payload []byte) (bitmapBytes []byte, pageData []byte, err error) {
	if len(payload) < 8 {
		return nil, nil, fmt.Errorf("%w: %d bytes", errDirtyPayloadTooShort, len(payload))
	}

	bitmapLen := binary.BigEndian.Uint64(payload[0:8])
	if uint64(len(payload)) < 8+bitmapLen {
		return nil, nil, errDirtyPayloadTruncated
	}

	return payload[8 : 8+bitmapLen], payload[8+bitmapLen:], nil
}

// bReader wraps a byte slice as an io.Reader.
type bReader []byte

func (b *bReader) Read(p []byte) (int, error) {
	if len(*b) == 0 {
		return 0, io.EOF
	}

	n := copy(p, *b)
	*b = (*b)[n:]

	return n, nil
}
