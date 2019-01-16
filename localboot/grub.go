package main

import (
	"io/ioutil"
	"log"
	"path"
	"strings"

	"github.com/systemboot/systemboot/pkg/bootconfig"
	"github.com/systemboot/systemboot/pkg/crypto"
)

// List of paths where to look for grub config files. Grub2Paths will look for
// files with grub2-compatible syntax, GrubLegacyPaths similarly will treat
// these as grub-legacy config files.
var (
	Grub2Paths = []string{
		// grub2
		"boot/grub2/grub.cfg",
		"boot/grub2.cfg",
		"grub2/grub.cfg",
		"grub2.cfg",
	}
	GrubLegacyPaths = []string{
		// grub legacy
		"boot/grub/grub.cfg",
		"boot/grub.cfg",
		"grub/grub.cfg",
		"grub.cfg",
	}
)

type grubVersion int

var (
	grubV1 grubVersion = 1
	grubV2 grubVersion = 2
)

// ParseGrubCfg parses the content of a grub.cfg and returns a list of
// BootConfig structures, one for each menuentry, in the same order as they
// appear in grub.cfg. All opened kernel and initrd files are relative to
// basedir.
func ParseGrubCfg(ver grubVersion, grubcfg string, basedir string) []bootconfig.BootConfig {
	// This parser sucks. It's not even a parser, it just looks for lines
	// starting with menuentry, linux or initrd.
	// TODO use a parser, e.g. https://github.com/alecthomas/participle
	if ver != grubV1 && ver != grubV2 {
		log.Printf("Warning: invalid GRUB version: %d", ver)
		return nil
	}
	bootconfigs := make([]bootconfig.BootConfig, 0)
	inMenuEntry := false
	var cfg *bootconfig.BootConfig
	for _, line := range strings.Split(grubcfg, "\n") {
		// remove all leading spaces as they are not relevant for the config
		// line
		line = strings.TrimLeft(line, " ")
		sline := strings.Fields(line)
		if len(sline) == 0 {
			continue
		}
		if sline[0] == "menuentry" {
			// if a "menuentry", start a new boot config
			if cfg != nil {
				// save the previous boot config, if any
				if cfg.IsValid() {
					// only consider valid boot configs, i.e. the ones that have
					// both kernel and initramfs
					bootconfigs = append(bootconfigs, *cfg)
				}
			}
			inMenuEntry = true
			cfg = new(bootconfig.BootConfig)
			name := strings.Join(sline[1:], " ")
			cfg.Name = name
		} else if inMenuEntry {
			// otherwise look for kernel and initramfs configuration
			if len(sline) < 2 {
				// surely not a valid linux or initrd directive, skip it
				continue
			}
			if sline[0] == "linux" || sline[0] == "linux16" || sline[0] == "linuxefi" {
				kernel := sline[1]
				cmdline := strings.Join(sline[2:], " ")
				cmdline = unquote(ver, cmdline)
				cfg.Kernel = path.Join(basedir, kernel)
				cfg.KernelArgs = cmdline
			} else if sline[0] == "initrd" || sline[0] == "initrd16" || sline[0] == "initrdefi" {
				initrd := sline[1]
				cfg.Initramfs = path.Join(basedir, initrd)
			} else if sline[0] == "multiboot" {
				multiboot := sline[1]
				cmdline := strings.Join(sline[2:], " ")
				cmdline = unquote(ver, cmdline)
				cfg.Multiboot = path.Join(basedir, multiboot)
				cfg.MultibootArgs = cmdline
			} else if sline[0] == "module" {
				module := sline[1]
				cmdline := strings.Join(sline[2:], " ")
				if ver == grubV2 {
					// if grub2, unquote the string, as directives could be quoted
					// https://www.gnu.org/software/grub/manual/grub/grub.html#Quoting
					// TODO unquote everything, not just \$
					cmdline = strings.Replace(cmdline, `\$`, "$", -1)
				}
				module = path.Join(basedir, module)
				if cmdline != "" {
					module = module + " " + cmdline
				}
				cfg.Modules = append(cfg.Modules, module)
			}
		}
	}
	// append last kernel config if it wasn't already
	if inMenuEntry && cfg.IsValid() {
		bootconfigs = append(bootconfigs, *cfg)
	}
	return bootconfigs
}

func unquote(ver grubVersion, text string) string {
	if ver == grubV2 {
		// if grub2, unquote the string, as directives could be quoted
		// https://www.gnu.org/software/grub/manual/grub/grub.html#Quoting
		// TODO unquote everything, not just \$
		return strings.Replace(text, `\$`, "$", -1)
	}
	// otherwise return the unmodified string
	return text
}

// ScanGrubConfigs looks for grub2 and grub legacy config files in the known
// locations and returns a list of boot configurations.
func ScanGrubConfigs(basedir string) []bootconfig.BootConfig {
	bootconfigs := make([]bootconfig.BootConfig, 0)
	// Scan Grub 2 configurations
	for _, grubpath := range Grub2Paths {
		path := path.Join(basedir, grubpath)
		log.Printf("Trying to read %s", path)
		grubcfg, err := ioutil.ReadFile(path)
		if err != nil {
			log.Printf("cannot open %s: %v", path, err)
			continue
		}
		crypto.TryMeasureData(crypto.ConfigData, grubcfg, path)
		cfgs := ParseGrubCfg(grubV2, string(grubcfg), basedir)
		bootconfigs = append(bootconfigs, cfgs...)
	}
	// Scan Grub Legacy configurations
	for _, grubpath := range GrubLegacyPaths {
		path := path.Join(basedir, grubpath)
		log.Printf("Trying to read %s", path)
		grubcfg, err := ioutil.ReadFile(path)
		if err != nil {
			log.Printf("cannot open %s: %v", path, err)
			continue
		}
		crypto.TryMeasureData(crypto.ConfigData, grubcfg, path)
		cfgs := ParseGrubCfg(grubV1, string(grubcfg), basedir)
		bootconfigs = append(bootconfigs, cfgs...)
	}
	return bootconfigs
}
