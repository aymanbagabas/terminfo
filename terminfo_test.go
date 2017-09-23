package terminfo

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestOpen(t *testing.T) {
	var fileRE = regexp.MustCompile("^([0-9]+|[a-zA-Z])/")

	for _, dir := range []string{"/lib/terminfo", "/usr/share/terminfo"} {
		t.Run(dir[1:], func(dir string) func(*testing.T) {
			return func(t *testing.T) {
				t.Parallel()
				werr := filepath.Walk(dir, func(file string, fi os.FileInfo, err error) error {
					if err != nil {
						return err
					}

					if fi.IsDir() || !fileRE.MatchString(file[len(dir)+1:]) {
						return nil
					}

					term := filepath.Base(file)

					// open
					ti, err := Open(dir, term)
					if err != nil {
						t.Fatalf("term %s expected no error, got: %v", term, err)
					}

					if ti.File != file {
						t.Errorf("term %s should have file %s, got: %s", term, file, ti.File)
					}

					// check we have at least one name
					if len(ti.Names) < 1 {
						t.Errorf("term %s expected names to have at least one value", term)
					}

					return nil
				})
				if werr != nil {
					t.Fatalf("could not walk directory, got: %v", werr)
				}
			}
		}(dir))
	}
}

var badTermAcscMap = map[string]bool{
	"rxvt-unicode-256color": true,
	"rxvt-cygwin-native":    true,
	"rxvt-unicode":          true,
	"hurd":                  true,
	"rxvt-cygwin":           true,
}

func TestValues(t *testing.T) {
	var fileRE = regexp.MustCompile("^([0-9]+|[a-zA-Z])/")

	terms := make(map[string]string)
	for _, dir := range []string{"/lib/terminfo", "/usr/share/terminfo"} {
		werr := filepath.Walk(dir, func(file string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() || !fileRE.MatchString(file[len(dir)+1:]) || fi.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			terms[filepath.Base(file)] = file
			return nil
		})
		if werr != nil {
			t.Fatalf("could not walk directory, got: %v", werr)
		}
	}

	var boolCount, numCount, stringCount int

	for term := range terms {
		if term == "xterm-old" {
			continue
		}

		ic, err := getInfocmpData(t, term)
		if err != nil {
			t.Fatalf("term %s could not load infocmp data, got: %v", term, err)
		}

		// load
		ti, err := Load(term)
		if err != nil {
			t.Fatalf("term %s expected no error, got: %v", term, err)
		}

		// check names
		if !reflect.DeepEqual(ic.names, ti.Names) {
			t.Errorf("term %s names do not match", term)
		}

		// check bool caps
		for i, v := range ic.boolCaps {
			if v == nil {
				if _, ok := ti.BoolsM[i]; !ok {
					t.Errorf("term %s expected bool cap %d (%s) to be missing", term, i, BoolCapName(i))
				}
			} else if v.(bool) != ti.Bools[i] {
				t.Errorf("term %s bool cap %d (%s) should be %t", term, i, BoolCapName(i), v)
			}
			boolCount++
		}

		// check num caps
		for i, v := range ic.numCaps {
			if v == nil {
				if _, ok := ti.NumsM[i]; !ok {
					//t.Errorf("term %s expected num cap %d (%s) to be missing", term, i, NumCapName(i))
				}
			} else if v.(int) != ti.Nums[i] {
				t.Errorf("term %s num cap %d (%s) should be %d", term, i, NumCapName(i), v)
			}
			numCount++
		}

		// check num caps
		for i, v := range ic.stringCaps {
			if i == AcsChars && badTermAcscMap[term] {
				continue
			}

			if v == nil {
				if _, ok := ti.StringsM[i]; !ok {
					//t.Errorf("term %s expected string cap %d (%s) to be missing", term, i, StringCapName(i))
				}
			} else if v.(string) != string(ti.Strings[i]) {
				t.Errorf("term %s string cap %d (%s) is invalid:", term, i, StringCapName(i))
				t.Errorf("got:  %#v", ti.Strings[i])
				t.Errorf("want: %#v", v)
			}
			stringCount++
		}
	}

	t.Logf("tested: %d terms, %d bools, %d nums, %d strings", len(terms), boolCount, numCount, stringCount)
}

var (
	shortCapNameMap map[string]int
)

func init() {
	shortCapNameMap = make(map[string]int)
	for _, z := range [][]string{boolCapNames[:], numCapNames[:], stringCapNames[:]} {
		for i := 0; i < len(z); i += 2 {
			shortCapNameMap[z[i+1]] = i / 2
		}
	}
}

var (
	boolSectRE   = regexp.MustCompile(`_bool_data\[\]\s*=\s*{`)
	numSectRE    = regexp.MustCompile(`_number_data\[\]\s*=\s*{`)
	stringSectRE = regexp.MustCompile(`_string_data\[\]\s*=\s*{`)
	endSectRE    = regexp.MustCompile(`(?m)^};$`)

	capValuesRE = regexp.MustCompile(`(?m)^\s+/\*\s+[0-9]+:\s+([^\s]+)\s+\*/\s+(.*),$`)
	numRE       = regexp.MustCompile(`^[0-9]+$`)
	absCanRE    = regexp.MustCompile(`^(ABSENT|CANCELLED)_(BOOLEAN|NUMERIC|STRING)$`)
)

type infocmp struct {
	names      []string
	boolCaps   map[int]interface{}
	numCaps    map[int]interface{}
	stringCaps map[int]interface{}
}

func processSect(buf []byte, stringCaps map[string]string, ic *infocmp, zz map[int]interface{}, sectRE *regexp.Regexp) error {
	var err error
	start := sectRE.FindIndex(buf)
	if start == nil || len(start) != 2 {
		return fmt.Errorf("could not find sect (%s)", sectRE)
	}
	end := endSectRE.FindIndex(buf[start[1]:])
	if end == nil || len(end) != 2 {
		return fmt.Errorf("could not find end of section (%s)", sectRE)
	}

	buf = buf[start[1] : start[1]+end[0]]

	// load caps
	m := capValuesRE.FindAllStringSubmatch(string(buf), -1)
	for i, s := range m {
		// get long cap name
		var k int
		var ok bool
		k, ok = shortCapNameMap[s[1]]
		if !ok {
			return fmt.Errorf("unknown  cap name '%s'", s[1])
		}

		// get cap value
		var v interface{}
		switch {
		case s[2] == "TRUE" || s[2] == "FALSE":
			v = s[2] == "TRUE"

		case numRE.MatchString(s[2]):
			var j int64
			j, err = strconv.ParseInt(s[2], 10, 16)
			if err != nil {
				return fmt.Errorf("line %d could not parse: %v", i, err)
			}
			v = int(j)

		case absCanRE.MatchString(s[2]):

		default:
			v, ok = stringCaps[s[2]]
			if !ok {
				return fmt.Errorf("cap '%s' not defined in cap table", s[2])
			}
		}

		zz[k] = v
	}

	return nil
}

var (
	staticCharRE = regexp.MustCompile(`(?m)^static\s+char\s+(.*)\s*\[\]\s*=\s*(".*");$`)
)

func getInfocmpData(t *testing.T, term string) (*infocmp, error) {
	c := exec.Command("/usr/bin/infocmp", "-E")
	c.Env = []string{"TERM=" + term}

	buf, err := c.CombinedOutput()
	if err != nil {
		t.Logf("shell error (TERM=%s):\n%s\n", term, string(buf))
		return nil, err
	}

	// read static strings
	m := staticCharRE.FindAllStringSubmatch(string(buf), -1)
	if !strings.HasSuffix(strings.TrimSpace(m[0][1]), "_alias_data") {
		return nil, errors.New("missing _alias_data")
	}

	// some names have " in them, and infocmp -E doesn't correctly escape them
	names, err := strconv.Unquote(`"` + strings.Replace(m[0][2][1:len(m[0][2])-1], `"`, `\"`, -1) + `"`)
	if err != nil {
		return nil, fmt.Errorf("could not unquote _alias_data: %v", err)
	}

	ic := &infocmp{
		names:      strings.Split(names, "|"),
		boolCaps:   make(map[int]interface{}),
		numCaps:    make(map[int]interface{}),
		stringCaps: make(map[int]interface{}),
	}

	// load string cap data
	stringCaps := make(map[string]string, len(m))
	for i, s := range m[1:] {
		k := strings.TrimSpace(s[1])
		idx := strings.LastIndex(k, "_s_")
		if idx == -1 {
			return nil, fmt.Errorf("string cap %d (%s) does not contain _s_", i, k)
		}

		v, err := strconv.Unquote(s[2])
		if err != nil {
			return nil, fmt.Errorf("could not unquote %d: %v", i, err)
		}
		stringCaps[k] = v
	}

	// extract the values
	for _, err := range []error{
		processSect(buf, stringCaps, ic, ic.boolCaps, boolSectRE),
		processSect(buf, stringCaps, ic, ic.numCaps, numSectRE),
		processSect(buf, stringCaps, ic, ic.stringCaps, stringSectRE),
	} {
		if err != nil {
			return nil, err
		}
	}

	return ic, nil
}
