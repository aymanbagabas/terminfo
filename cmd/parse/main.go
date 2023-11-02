package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/xo/terminfo"
)

func init() {
	flag.Usage = func() {
		os.Stderr.WriteString("Usage: parse sourcefile [terminfo to print]...\n")
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	data, err := os.ReadFile(args[0])
	if err != nil {
		log.Fatal(err)
	}

	tis, err := terminfo.Parse(data)
	if err != nil {
		log.Fatal(err)
	}

	printterms := flag.Args()[1:]

	tim := terminfo.NewTerminfos(tis)
	for k, v := range tim {
		for _, p := range printterms {
			if k == p {
				printti(v)
			}
		}
	}
}

func printti(ti *terminfo.Terminfo) {
	names := make([]string, 0)
	values := make(map[string]interface{})
	for i, v := range ti.ExtBoolNames {
		name := string(v)
		names = append(names, name, name)
		values[name] = ti.ExtBools[i]
	}
	for i, v := range ti.ExtNumNames {
		name := string(v)
		names = append(names, name, name)
		values[name] = ti.ExtNums[i]
	}
	for i, v := range ti.ExtStringNames {
		name := string(v)
		names = append(names, name, name)
		values[name] = string(ti.ExtStrings[i])
	}
	for i, v := range ti.Bools {
		name := terminfo.BoolCapNameShort(i)
		names = append(names, name)
		values[name] = v
	}
	for i, v := range ti.Nums {
		name := terminfo.NumCapNameShort(i)
		names = append(names, name)
		values[name] = v
	}
	for i, v := range ti.Strings {
		name := terminfo.StringCapNameShort(i)
		names = append(names, name)
		values[name] = string(v)
	}
	sort.Strings(names)
	for _, n := range names {
		format := "\t%s="
		value := values[n]
		switch value.(type) {
		case bool:
			format += "%t"
		case int:
			format += "%d"
		case string:
			format += "%q"
		default:
			format += "%v"
		}
		fmt.Printf(format+"\n", n, value)
	}
}
