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
//go:generate stringer -type=F_1_Edx,F_7_0_Edx -output=features_string.go
type Feature interface {
	F_1_Edx | F_7_0_Edx

	fmt.Stringer
}

type (
	F_1_Edx   uint32
	F_7_0_Edx uint32
)

const (
	FPU       F_1_Edx = 0  /* Onboard FPU */
	VME       F_1_Edx = 1  /* Virtual Mode Extensions */
	DE        F_1_Edx = 2  /* Debugging Extensions */
	PSE       F_1_Edx = 3  /* Page Size Extensions */
	TSC       F_1_Edx = 4  /* Time Stamp Counter */
	MSR       F_1_Edx = 5  /* Model-Specific Registers */
	PAE       F_1_Edx = 6  /* Physical Address Extensions */
	MCE       F_1_Edx = 7  /* Machine Check Exception */
	CX8       F_1_Edx = 8  /* CMPXCHG8 instruction */
	APIC      F_1_Edx = 9  /* Onboard APIC */
	SEP       F_1_Edx = 11 /* SYSENTER/SYSEXIT */
	MTRR      F_1_Edx = 12 /* Memory Type Range Registers */
	PGE       F_1_Edx = 13 /* Page Global Enable */
	MCA       F_1_Edx = 14 /* Machine Check Architecture */
	CMOV      F_1_Edx = 15 /* CMOV instructions (plus FCMOVcc, FCOMI with FPU) */
	PAT       F_1_Edx = 16 /* Page Attribute Table */
	PSE36     F_1_Edx = 17 /* 36-bit PSEs */
	PN        F_1_Edx = 18 /* Processor serial number */
	CLFLUSH   F_1_Edx = 19 /* CLFLUSH instruction */
	DS        F_1_Edx = 21 /* "dts" Debug Store */
	ACPI      F_1_Edx = 22 /* ACPI via MSR */
	MMX       F_1_Edx = 23 /* Multimedia Extensions */
	FXSR      F_1_Edx = 24 /* FXSAVE/FXRSTOR, CR4.OSFXSR */
	XMM       F_1_Edx = 25 /* "sse" */
	XMM2      F_1_Edx = 26 /* "sse2" */
	SELFSNOOP F_1_Edx = 27 /* "ss" CPU self snoop */
	HT        F_1_Edx = 28 /* Hyper-Threading */
	ACC       F_1_Edx = 29 /* "tm" Automatic clock control */
	IA64      F_1_Edx = 30 /* IA-64 processor */
	PBE       F_1_Edx = 31 /* Pending Break Enable */
)

const (
	AVX512_4VNNIW       F_7_0_Edx = 2  /* AVX-512 Neural Network Instructions */
	AVX512_4FMAPS       F_7_0_Edx = 3  /* AVX-512 Multiply Accumulation Single precision */
	FSRM                F_7_0_Edx = 4  /* Fast Short Rep Mov */
	AVX512_VP2INTERSECT F_7_0_Edx = 8  /* AVX-512 Intersect for D/Q */
	SRBDS_CTRL          F_7_0_Edx = 9  /* "" SRBDS mitigation MSR available */
	MD_CLEAR            F_7_0_Edx = 10 /* VERW clears CPU buffers */
	RTM_ALWAYS_ABORT    F_7_0_Edx = 11 /* "" RTM transaction always aborts */
	TSX_FORCE_ABORT     F_7_0_Edx = 13 /* "" TSX_FORCE_ABORT */
	SERIALIZE           F_7_0_Edx = 14 /* SERIALIZE instruction */
	HYBRID_CPU          F_7_0_Edx = 15 /* "" This part has CPUs of more than one type */
	TSXLDTRK            F_7_0_Edx = 16 /* TSX Suspend Load Address Tracking */
	PCONFIG             F_7_0_Edx = 18 /* Intel PCONFIG */
	ARCH_LBR            F_7_0_Edx = 19 /* Intel ARCH LBR */
	IBT                 F_7_0_Edx = 20 /* Indirect Branch Tracking */
	AMX_BF16            F_7_0_Edx = 22 /* AMX bf16 Support */
	AVX512_FP16         F_7_0_Edx = 23 /* AVX512 FP16 */
	AMX_TILE            F_7_0_Edx = 24 /* AMX tile Support */
	AMX_INT8            F_7_0_Edx = 25 /* AMX int8 Support */
	SPEC_CTRL           F_7_0_Edx = 26 /* "" Speculation Control (IBRS + IBPB) */
	INTEL_STIBP         F_7_0_Edx = 27 /* "" Single Thread Indirect Branch Predictors */
	FLUSH_L1D           F_7_0_Edx = 28 /* Flush L1D cache */
	ARCH_CAPABILITIES   F_7_0_Edx = 29 /* IA32_ARCH_CAPABILITIES MSR (Intel) */
	CORE_CAPABILITIES   F_7_0_Edx = 30 /* "" IA32_CORE_CAPABILITIES MSR */
	SPEC_CTRL_SSBD      F_7_0_Edx = 31 /* "" Speculative Store Bypass Disable */
)

var All_F_1_Edx = []F_1_Edx{
	FPU, VME, DE, PSE, TSC, MSR, PAE, MCE, CX8, APIC, SEP, MTRR, PGE, MCA,
	CMOV, PAT, PSE36, PN, CLFLUSH, DS, ACPI, MMX, FXSR, XMM, XMM2,
	SELFSNOOP, HT, ACC, IA64, PBE,
}

var All_F_7_0_Edx = []F_7_0_Edx{
	AVX512_4VNNIW, AVX512_4FMAPS, FSRM, AVX512_VP2INTERSECT, SRBDS_CTRL,
	MD_CLEAR, RTM_ALWAYS_ABORT, TSX_FORCE_ABORT, SERIALIZE, HYBRID_CPU,
	TSXLDTRK, PCONFIG, ARCH_LBR, IBT, AMX_BF16, AVX512_FP16, AMX_TILE,
	AMX_INT8, SPEC_CTRL, INTEL_STIBP, FLUSH_L1D, ARCH_CAPABILITIES,
	CORE_CAPABILITIES, SPEC_CTRL_SSBD,
}
