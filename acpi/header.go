package acpi

type Header struct {
	Signature  [4]byte
	Length     uint32
	Rev        uint8
	Checksum   uint8
	OEMId      [6]byte
	OEMTableID [8]byte
	OEMRev     uint32
	CreatorID  [4]byte
	CreatorRev uint32
}

func convertOEMID(oemID string) [6]byte {
	var id [6]byte

	for i := 0; i < 6; i++ {
		id[i] = oemID[i]
	}

	return id
}

func convertOEMTableID(oemTableID string) [8]byte {
	var id [8]byte

	for i := 0; i < 8; i++ {
		id[i] = oemTableID[i]
	}

	return id
}

func convertCreatorID(creatorID string) [4]byte {
	var id [4]byte

	for i := 0; i < 4; i++ {
		id[i] = creatorID[i]
	}

	return id
}

func newHeader(sig Signature, length uint32, rev uint8, oemID, oemTableID string) Header {
	creatorID := "GACT" // Go ACPI Tables.

	oid := convertOEMID(oemID)
	otid := convertOEMTableID(oemTableID)
	cid := convertCreatorID(creatorID)

	return Header{
		Signature:  sig.ToBytes(),
		Length:     length,
		Rev:        rev,
		OEMId:      oid,
		OEMTableID: otid,
		CreatorID:  cid,
		CreatorRev: 1,
	}
}
