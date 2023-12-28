package cpuid

import "fmt"

// The list of CPU features can be found in arch/x86/kvm/cpuid.c [1]
// in Linux. Also in ths same file, the relationship between CPU features and
// CPUID functions [2] are defined. The offset in the register is defined in
// arch/x86/include/asm/cpufeatures.h [3].
//
// [1] https://github.com/torvalds/linux/blob/v4.20/arch/x86/kvm/cpuid.c#L341-L414
// [2] https://github.com/torvalds/linux/blob/v4.20/arch/x86/kvm/cpuid.c#L427-L513
// [3] https://github.com/torvalds/linux/blob/v4.20/arch/x86/include/asm/cpufeatures.h#L29

// The unifed interface which contains all CPU features.
//
//go:generate stringer -type=F1Edx,F7_0Edx -output=features_string.go
type Feature interface {
	F1Edx | F7_0Edx

	fmt.Stringer
}

type (
	F1Edx   uint32
	F7_0Edx uint32
)

const (
	FPU       F1Edx = 0  /* Onboard FPU */
	VME       F1Edx = 1  /* Virtual Mode Extensions */
	DE        F1Edx = 2  /* Debugging Extensions */
	PSE       F1Edx = 3  /* Page Size Extensions */
	TSC       F1Edx = 4  /* Time Stamp Counter */
	MSR       F1Edx = 5  /* Model-Specific Registers */
	PAE       F1Edx = 6  /* Physical Address Extensions */
	MCE       F1Edx = 7  /* Machine Check Exception */
	CX8       F1Edx = 8  /* CMPXCHG8 instruction */
	APIC      F1Edx = 9  /* Onboard APIC */
	SEP       F1Edx = 11 /* SYSENTER/SYSEXIT */
	MTRR      F1Edx = 12 /* Memory Type Range Registers */
	PGE       F1Edx = 13 /* Page Global Enable */
	MCA       F1Edx = 14 /* Machine Check Architecture */
	CMOV      F1Edx = 15 /* CMOV instructions (plus FCMOVcc, FCOMI with FPU) */
	PAT       F1Edx = 16 /* Page Attribute Table */
	PSE36     F1Edx = 17 /* 36-bit PSEs */
	PN        F1Edx = 18 /* Processor serial number */
	CLFLUSH   F1Edx = 19 /* CLFLUSH instruction */
	DS        F1Edx = 21 /* "dts" Debug Store */
	ACPI      F1Edx = 22 /* ACPI via MSR */
	MMX       F1Edx = 23 /* Multimedia Extensions */
	FXSR      F1Edx = 24 /* FXSAVE/FXRSTOR, CR4.OSFXSR */
	XMM       F1Edx = 25 /* "sse" */
	XMM2      F1Edx = 26 /* "sse2" */
	SELFSNOOP F1Edx = 27 /* "ss" CPU self snoop */
	HT        F1Edx = 28 /* Hyper-Threading */
	ACC       F1Edx = 29 /* "tm" Automatic clock control */
	IA64      F1Edx = 30 /* IA-64 processor */
	PBE       F1Edx = 31 /* Pending Break Enable */
)

//nolint:stylecheck
const (
	AVX512_4VNNIW       F7_0Edx = 2  /* AVX-512 Neural Network Instructions */
	AVX512_4FMAPS       F7_0Edx = 3  /* AVX-512 Multiply Accumulation Single precision */
	FSRM                F7_0Edx = 4  /* Fast Short Rep Mov */
	AVX512_VP2INTERSECT F7_0Edx = 8  /* AVX-512 Intersect for D/Q */
	SRBDS_CTRL          F7_0Edx = 9  /* "" SRBDS mitigation MSR available */
	MD_CLEAR            F7_0Edx = 10 /* VERW clears CPU buffers */
	RTM_ALWAYS_ABORT    F7_0Edx = 11 /* "" RTM transaction always aborts */
	TSX_FORCE_ABORT     F7_0Edx = 13 /* "" TSX_FORCE_ABORT */
	SERIALIZE           F7_0Edx = 14 /* SERIALIZE instruction */
	HYBRID_CPU          F7_0Edx = 15 /* "" This part has CPUs of more than one type */
	TSXLDTRK            F7_0Edx = 16 /* TSX Suspend Load Address Tracking */
	PCONFIG             F7_0Edx = 18 /* Intel PCONFIG */
	ARCH_LBR            F7_0Edx = 19 /* Intel ARCH LBR */
	IBT                 F7_0Edx = 20 /* Indirect Branch Tracking */
	AMX_BF16            F7_0Edx = 22 /* AMX bf16 Support */
	AVX512_FP16         F7_0Edx = 23 /* AVX512 FP16 */
	AMX_TILE            F7_0Edx = 24 /* AMX tile Support */
	AMX_INT8            F7_0Edx = 25 /* AMX int8 Support */
	SPEC_CTRL           F7_0Edx = 26 /* "" Speculation Control (IBRS + IBPB) */
	INTEL_STIBP         F7_0Edx = 27 /* "" Single Thread Indirect Branch Predictors */
	FLUSH_L1D           F7_0Edx = 28 /* Flush L1D cache */
	ARCH_CAPABILITIES   F7_0Edx = 29 /* IA32_ARCH_CAPABILITIES MSR (Intel) */
	CORE_CAPABILITIES   F7_0Edx = 30 /* "" IA32_CORE_CAPABILITIES MSR */
	SPEC_CTRL_SSBD      F7_0Edx = 31 /* "" Speculative Store Bypass Disable */
)

//nolint:gochecknoglobals
var AllF1Edx = []F1Edx{
	FPU, VME, DE, PSE, TSC, MSR, PAE, MCE, CX8, APIC, SEP, MTRR, PGE, MCA,
	CMOV, PAT, PSE36, PN, CLFLUSH, DS, ACPI, MMX, FXSR, XMM, XMM2,
	SELFSNOOP, HT, ACC, IA64, PBE,
}

//nolint:gochecknoglobals
var AllF7_0Edx = []F7_0Edx{
	AVX512_4VNNIW, AVX512_4FMAPS, FSRM, AVX512_VP2INTERSECT, SRBDS_CTRL,
	MD_CLEAR, RTM_ALWAYS_ABORT, TSX_FORCE_ABORT, SERIALIZE, HYBRID_CPU,
	TSXLDTRK, PCONFIG, ARCH_LBR, IBT, AMX_BF16, AVX512_FP16, AMX_TILE,
	AMX_INT8, SPEC_CTRL, INTEL_STIBP, FLUSH_L1D, ARCH_CAPABILITIES,
	CORE_CAPABILITIES, SPEC_CTRL_SSBD,
}
