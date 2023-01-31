package kvm

import "errors"

var (
	// ErrUnexpectedExitReason is any error that we do not understand.
	ErrUnexpectedExitReason = errors.New("unexpected kvm exit reason")

	// ErrDebug is a debug exit, caused by single step or breakpoint.
	ErrDebug = errors.New("debug exit")
)

// ExitType is a virtual machine exit type.
//
//go:generate stringer -type=ExitType
type ExitType uint

const (
	EXITUNKNOWN       ExitType = 0
	EXITEXCEPTION     ExitType = 1
	EXITIO            ExitType = 2
	EXITHYPERCALL     ExitType = 3
	EXITDEBUG         ExitType = 4
	EXITHLT           ExitType = 5
	EXITMMIO          ExitType = 6
	EXITIRQWINDOWOPEN ExitType = 7
	EXITSHUTDOWN      ExitType = 8
	EXITFAILENTRY     ExitType = 9
	EXITINTR          ExitType = 10
	EXITSETTPR        ExitType = 11
	EXITTPRACCESS     ExitType = 12
	EXITS390SIEIC     ExitType = 13
	EXITS390RESET     ExitType = 14
	EXITDCR           ExitType = 15
	EXITNMI           ExitType = 16
	EXITINTERNALERROR ExitType = 17

	EXITIOIN  = 0
	EXITIOOUT = 1
)
