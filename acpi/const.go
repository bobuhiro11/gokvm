package acpi

type Signature string

func (s Signature) ToBytes() [4]byte {
	var ret [4]byte

	for i := 0; i < 3; i++ {
		ret[i] = s[i]
	}

	return ret
}

const (
	SigAEST Signature = "AEST"
	SigAPIC Signature = "APIC"
	SigBDAT Signature = "BDAT"
	SigBERT Signature = "BERT"
	SigBGRT Signature = "BGRT"
	SigBOOT Signature = "BOOT"
	SigCDIT Signature = "CDIT"
	SigCEDT Signature = "CEDT"
	SigCPEP Signature = "CPEP"
	SigCRAT Signature = "CRAT"
	SigCSRT Signature = "CSRT"
	SigDBGP Signature = "DBGP"
	SigDBG2 Signature = "DBG2"
	SigDMAR Signature = "DMAR"
	SigDRTM Signature = "DRTM"
	SigDSDT Signature = "DSDT"
	SigECDT Signature = "ECDT"
	SigETDT Signature = "ETDT"
	SigEINJ Signature = "EINJ"
	SigERST Signature = "ERST"
	SigFACP Signature = "FACP"
	SigFACS Signature = "FACS"
	SigFPDT Signature = "FPDT"
	SigGTDT Signature = "GTDT"
	SigHPET Signature = "HPET"
	SigHEST Signature = "HEST"
	SigIBFT Signature = "IBFT"
	SigIORT Signature = "IORT"
	SigIVRS Signature = "IVRS"
	SigLPIT Signature = "LPIT"
	SigMCFG Signature = "MCFG"
	SigMCHI Signature = "MCHI"
	SigMPAM Signature = "MPAM"
	SigMSCT Signature = "MSCT"
	SigMSDM Signature = "MSDM"
	SigMPST Signature = "MPST"
	SigNFIT Signature = "NFIT"
	SigOEMx Signature = "OEMx"
	SigPCCT Signature = "PCCT"
	SigPHAT Signature = "PHAT"
	SigPMTT Signature = "PMTT"
	SigPRMT Signature = "PRMT"
	SigPSDT Signature = "PSDT"
	SigRASF Signature = "RASF"
	SigRGRT Signature = "RGRT"
	SigRSDT Signature = "RSDT"
	SigSBST Signature = "SBST"
	SigSDEI Signature = "SDEI"
	SigSDEV Signature = "SDEV"
	SigSLIC Signature = "SLIC"
	SigSLIT Signature = "SLIT"
	SigSPCR Signature = "SPCR"
	SigSPMI Signature = "SPMI"
	SigSRAT Signature = "SRAT"
	SigSSDT Signature = "SSDT"
	SigSTAO Signature = "STAO"
	SigSVKL Signature = "SVKL"
	SigTCPA Signature = "TCPA"
	SigTPM2 Signature = "TPM2"
	SigUEFI Signature = "UEFI"
	SigVIOT Signature = "VIOT"
	SigWAET Signature = "WAET"
	SigWDAT Signature = "WDAT"
	SigWDRT Signature = "WDRT"
	SigWDBT Signature = "WDBT"
	SigWSMT Signature = "WSMT"
	SigXENV Signature = "XENV"
	SigXSDT Signature = "XSDT"
)
