// A gist from Drahoslav Bednář with some additions like initialisation only and output to a file
// https://gist.github.com/drahoslove/0342e1de9847805a5a12e260dd178e82

// +build ignore

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

type Format int

const (
	hexx  Format = iota // 0x00, 0x01, 0x6b
	hex                 // 0x0, 0x1, 0x6b
	dec                 // 0, 1, 107
	shexx               // "\x00\x01\x6b",
	shex                // "\x00\x01k"
)

func (f Format) String() string {
	return map[Format]string{
		hexx:  "hexx",
		hex:   "hex",
		dec:   "dec",
		shexx: "shexx",
		shex:  "shex",
	}[f]
}
func (f *Format) Set(name string) error {
	val, ok := map[string]Format{
		"hexx":  hexx,
		"hex":   hex,
		"dec":   dec,
		"shexx": shexx,
		"shex":  shex,
	}[name]
	if !ok {
		return fmt.Errorf("may be: hexx, hex, dec, shexx, or shex")
	}
	*f = val
	return nil
}

var (
	packageName = "main"
	varName     = "_"
	format      = hexx
	step        = 16
	initOnly    = false
	fileOutName = ""
	fileInName  = ""
)

func init() {
	flag.StringVar(&packageName, "package", packageName, "package `name`")
	flag.StringVar(&varName, "var", varName, "variable `name`")
	flag.IntVar(&step, "step", step, "`number` of bytes per line")
	flag.Var(&format, "format", "type of byte representation")
	flag.BoolVar(&initOnly, "init-only", initOnly, "no declaration of variable, initialization only")
	flag.StringVar(&fileOutName, "o", fileOutName, "output file name")
	flag.Parse()

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s [ flags ] FILENAME\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	fileInName = flag.Arg(0)
}

func main() {
	if fileInName == "" {
		flag.Usage()
		return
	}
	var buffer = read(fileInName) // TODO use reader instead of reading whole file at once

	parents := getParents(format)
	getLine := getGetLine(format)

	// Open output file
	f := os.Stdout
	if fileOutName != "" {
		var err error
		f, err = os.Create(fileOutName)
		if err != nil {
			log.Fatalf("Could not open output file: %v", err)
		}
		defer f.Close()
	}
	w := bufio.NewWriter(f)

	// Print out generated go source
	indent := ""
	fmt.Fprintf(w, "// Code generated by gobin.go DO NOT EDIT.\n")
	fmt.Fprintf(w, "package %s\n", packageName)
	fmt.Fprintf(w, "\n")
	if initOnly {
		fmt.Fprintln(w, "func init() {")
		indent = "\t"
	} else {
		fmt.Fprint(w, "var ")
	}
	fmt.Fprintf(w, "%s%s = []byte%c\n", indent, varName, parents[0])
	for i := 0; i < len(buffer); i += step {
		from, to := i, min(i+step, len(buffer))
		fmt.Fprintln(w, indent, getLine(buffer[from:to]))
	}
	fmt.Fprintf(w, "%s%c\n", indent, parents[1])
	if initOnly {
		fmt.Fprintln(w, "}")
	}
	w.Flush()
}

func read(fileName string) []byte {
	filecontent, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatal(err)
		return []byte{}
	}
	return filecontent
}

func getParents(format Format) string {
	switch format {
	case shexx, shex:
		return "()"
	case hex, hexx, dec:
		return "{}"
	}
	return ""
}

func getGetLine(format Format) func(buffer []byte) string {
	switch format {
	case hexx:
		return func(buffer []byte) string {
			items := make([]string, len(buffer))
			for i, _ := range buffer {
				items[i] = fmt.Sprintf("%#x", buffer[i:i+1])
			}
			return fmt.Sprintf("\t%s,", strings.Join(items, ", "))
		}
	case hex:
		return func(buffer []byte) string {
			items := make([]string, len(buffer))
			for i, b := range buffer {
				items[i] = fmt.Sprintf("%#x", b)
			}
			return fmt.Sprintf("\t%s,", strings.Join(items, ", "))
		}
	case dec:
		return func(buffer []byte) string {
			items := make([]string, len(buffer))
			for i, b := range buffer {
				items[i] = fmt.Sprintf("%d", b)
			}
			return fmt.Sprintf("\t%s,", strings.Join(items, ", "))
		}
	case shexx:
		return func(buffer []byte) string {
			delim := "+"
			if len(buffer) == cap(buffer) {
				delim = ","
			}
			items := make([]string, len(buffer))
			for i, _ := range buffer {
				items[i] = fmt.Sprintf("\\x%x", buffer[i:i+1])
			}
			return fmt.Sprintf("\t\"%s\"%s", strings.Join(items, ""), delim)
		}
	case shex:
		return func(buffer []byte) string {
			delim := "+"
			if len(buffer) == cap(buffer) {
				delim = ","
			}
			return fmt.Sprintf("\t%+q%s", buffer, delim)
		}
	}
	return func(buffer []byte) string { return "" }
}

func min(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}
