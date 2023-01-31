package machine

const (
	bootParamAddr = 0x10000
	cmdlineAddr   = 0x20000

	initrdAddr  = 0xf000000
	highMemBase = 0x100000

	serialIRQ    = 4
	virtioNetIRQ = 9
	virtioBlkIRQ = 10

	pageTableBase = 0x30_000

	MinMemSize = 1 << 25
)

const (
	// These *could* be in kvm, but we'll see.

	// golangci-lint is completely wrong about these names.
	// Control Register Paging Enable for example:
	// golang style requires all letters in an acronym to be caps.
	// CR0 bits.
	CR0xPE = 1
	CR0xMP = (1 << 1)
	CR0xEM = (1 << 2)
	CR0xTS = (1 << 3)
	CR0xET = (1 << 4)
	CR0xNE = (1 << 5)
	CR0xWP = (1 << 16)
	CR0xAM = (1 << 18)
	CR0xNW = (1 << 29)
	CR0xCD = (1 << 30)
	CR0xPG = (1 << 31)

	// CR4 bits.
	CR4xVME        = 1
	CR4xPVI        = (1 << 1)
	CR4xTSD        = (1 << 2)
	CR4xDE         = (1 << 3)
	CR4xPSE        = (1 << 4)
	CR4xPAE        = (1 << 5)
	CR4xMCE        = (1 << 6)
	CR4xPGE        = (1 << 7)
	CR4xPCE        = (1 << 8)
	CR4xOSFXSR     = (1 << 8)
	CR4xOSXMMEXCPT = (1 << 10)
	CR4xUMIP       = (1 << 11)
	CR4xVMXE       = (1 << 13)
	CR4xSMXE       = (1 << 14)
	CR4xFSGSBASE   = (1 << 16)
	CR4xPCIDE      = (1 << 17)
	CR4xOSXSAVE    = (1 << 18)
	CR4xSMEP       = (1 << 20)
	CR4xSMAP       = (1 << 21)

	EFERxSCE = 1
	EFERxLME = (1 << 8)
	EFERxLMA = (1 << 10)
	EFERxNXE = (1 << 11)

	// 64-bit page * entry bits.
	PDE64xPRESENT  = 1
	PDE64xRW       = (1 << 1)
	PDE64xUSER     = (1 << 2)
	PDE64xACCESSED = (1 << 5)
	PDE64xDIRTY    = (1 << 6)
	PDE64xPS       = (1 << 7)
	PDE64xG        = (1 << 8)
)

const (
	// Poison is an instruction that should force a vmexit.
	// it fills memory to make catching guest errors easier.
	// vmcall, nop is this pattern
	// Poison = []byte{0x0f, 0x0b, } //0x01, 0xC1, 0x90}
	// Disassembly:
	// 0:  b8 be ba fe ca          mov    eax,0xcafebabe
	// 5:  90                      nop
	// 6:  0f 0b                   ud2
	Poison = "\xB8\xBE\xBA\xFE\xCA\x90\x0F\x0B"
)
