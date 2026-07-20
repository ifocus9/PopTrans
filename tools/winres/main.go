package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tc-hib/winres"
)

func main() {
	iconPath := flag.String("icon", "", "path to the source ICO file")
	outputPath := flag.String("output", "", "path to the generated .syso file")
	archName := flag.String("arch", "amd64", "target architecture")
	flag.Parse()

	if *iconPath == "" || *outputPath == "" {
		exitf("-icon and -output are required")
	}

	arch, ok := map[string]winres.Arch{
		"386":   winres.ArchI386,
		"amd64": winres.ArchAMD64,
		"arm64": winres.ArchARM64,
	}[*archName]
	if !ok {
		exitf("unsupported architecture %q", *archName)
	}

	iconFile, err := os.Open(*iconPath)
	if err != nil {
		exitf("open icon: %v", err)
	}
	defer iconFile.Close()

	icon, err := winres.LoadICO(iconFile)
	if err != nil {
		exitf("load icon: %v", err)
	}

	resources := winres.ResourceSet{}
	if err := resources.SetIcon(winres.RT_ICON, icon); err != nil {
		exitf("set icon resource: %v", err)
	}

	output, err := os.Create(*outputPath)
	if err != nil {
		exitf("create resource file: %v", err)
	}

	if err := resources.WriteObject(output, arch); err != nil {
		_ = output.Close()
		_ = os.Remove(*outputPath)
		exitf("write resource file: %v", err)
	}
	if err := output.Close(); err != nil {
		_ = os.Remove(*outputPath)
		exitf("close resource file: %v", err)
	}
}

func exitf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
