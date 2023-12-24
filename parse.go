package terminfo

import (
	"bytes"
	"log"
	"strconv"
	"strings"
	"unicode"
)

type Terminfos map[string]*Terminfo

func NewTerminfos(tis []*Terminfo) Terminfos {
	tim := make(Terminfos)
	for _, ti := range tis {
		tim.Set(ti)
	}
	return tim
}

func (tis Terminfos) Find(name string) *Terminfo {
	return tis[name]
}

func (tis Terminfos) Set(ti *Terminfo) {
	for _, n := range ti.Names {
		tis[n] = ti
	}
}

// Parse parses the terminfo source file and returns the resulting terminfo
// terminal campabilities.
func Parse(data []byte) ([]*Terminfo, error) {
	tis := make([]*Terminfo, 0)
	src := string(data)

	var (
		ti      *Terminfo
		capName string
		esc     = GROUND
		buf     bytes.Buffer
	)

	extBoolIdx := 0
	extNumIdx := 0
	extStringIdx := 0
	extBoolNameCaps := make(map[string]int)
	extNumNameCaps := make(map[string]int)
	extStringNameCaps := make(map[string]int)

	addCap := func(typ string) {
		switch typ {
		case "bool":
			name := buf.String()
			if cap, ok := boolNameCaps[name]; ok {
				ti.Bools[cap] = true
			} else if cap, ok := extBoolNameCaps[name]; ok {
				ti.ExtBoolNames[cap] = []byte(name)
				ti.ExtBools[cap] = true
			} else {
				extBoolNameCaps[name] = extBoolIdx
				ti.ExtBoolNames[extBoolIdx] = []byte(name)
				ti.ExtBools[extBoolIdx] = true
				extBoolIdx++
			}
		case "num":
			value := buf.String()
			base := 10
			if strings.HasPrefix(value, "0x") {
				base = 16
				value = value[2:]
			}

			n, err := strconv.ParseUint(value, base, 32)
			if err != nil {
				log.Printf("Warn: invalid number: %q", value)
			}

			if cap, ok := numNameCaps[capName]; ok {
				ti.Nums[cap] = int(n)
			} else if cap, ok := extNumNameCaps[capName]; ok {
				ti.ExtNumNames[cap] = []byte(capName)
				ti.ExtNums[cap] = int(n)
			} else {
				extNumNameCaps[capName] = extNumIdx
				ti.ExtNumNames[extNumIdx] = []byte(capName)
				ti.ExtNums[extNumIdx] = int(n)
				extNumIdx++
			}
			capName = ""
		case "str":
			value := buf.String()
			if capName == "use" {
				ti.Uses = append(ti.Uses, value)
			} else {
				if cap, ok := stringNameCaps[capName]; ok {
					ti.Strings[cap] = []byte(value)
				} else if cap, ok := extStringNameCaps[capName]; ok {
					ti.ExtStringNames[cap] = []byte(capName)
					ti.ExtStrings[cap] = []byte(value)
				} else {
					extStringNameCaps[capName] = extStringIdx
					ti.ExtStringNames[extStringIdx] = []byte(capName)
					ti.ExtStrings[extStringIdx] = []byte(value)
					extStringIdx++
				}
			}
			capName = ""
		default:
			panic("WTF! who are you?")
		}
		buf.Reset()
		esc = GROUND
	}

	for _, line := range strings.Split(src, "\n") {
		switch {
		case strings.HasPrefix(line, "#"):
			fallthrough
		case strings.TrimSpace(line) == "":
			continue
		}

		parts := strings.Split(line, ",")
		if !unicode.IsSpace(rune(line[0])) {
			if ti != nil {
				tis = append(tis, ti)
			}
			names := strings.Split(parts[0], "|")
			ti = &Terminfo{
				Names:          names,
				Bools:          make(map[int]bool),
				Nums:           make(map[int]int),
				Strings:        make(map[int][]byte),
				BoolsM:         make(map[int]bool),
				NumsM:          make(map[int]bool),
				StringsM:       make(map[int]bool),
				ExtBools:       make(map[int]bool),
				ExtNums:        make(map[int]int),
				ExtStrings:     make(map[int][]byte),
				ExtBoolNames:   make(map[int][]byte),
				ExtNumNames:    make(map[int][]byte),
				ExtStringNames: make(map[int][]byte),
			}
		} else {
			s := strings.TrimSpace(line)
			for i := 0; i < len(s); i++ {
				c := s[i]
				switch esc {
				case GROUND:
					switch c {
					case '=':
						capName = buf.String()
						buf.Reset()
						esc = NONE
					case '#':
						capName = buf.String()
						buf.Reset()
						esc = INT
					case ',':
						if capName == "" {
							addCap("bool")
						} else {
							log.Printf("Shouldn't be here: %s", capName)
						}
						buf.Reset()
					case ' ':
						continue
					default:
						buf.WriteByte(c)
					}
				case INT:
					switch c {
					case ',':
						addCap("num")
						esc = GROUND
					default:
						buf.WriteByte(c)
					}
				case NONE:
					switch c {
					case '\\':
						esc = ESC
					case '^':
						esc = CTRL
					case ' ':
						continue
					case ',':
						addCap("str")
						esc = GROUND
					default:
						buf.WriteByte(c)
					}
				case CTRL:
					buf.WriteByte(c ^ 1<<6)
					esc = NONE
				case ESC:
					switch c {
					case 'E', 'e':
						buf.WriteByte(0x1b)
					case '0', '1', '2', '3', '4', '5', '6', '7':
						if i+2 < len(s) && s[i+1] >= '0' && s[i+1] <= '7' && s[i+2] >= '0' && s[i+2] <= '7' {
							buf.WriteByte(((c - '0') * 64) + ((s[i+1] - '0') * 8) + (s[i+2] - '0'))
							i = i + 2
						} else if c == '0' {
							buf.WriteByte(0)
						}
					case 'n':
						buf.WriteByte('\n')
					case 'r':
						buf.WriteByte('\r')
					case 't':
						buf.WriteByte('\t')
					case 'b':
						buf.WriteByte('\b')
					case 'f':
						buf.WriteByte('\f')
					case 's':
						buf.WriteByte(' ')
					case ',':
						buf.WriteByte(',')
					case 'l':
						panic("WTF: weird format: " + s)
					default:
						buf.WriteByte(c)
					}
					esc = NONE
				}
			}
		}
	}

	// Append d the last terminfo
	if ti != nil {
		tis = append(tis, ti)
	}

	// Resolve uses
	for i, ti := range tis {
		if len(ti.Uses) == 0 {
			continue
		}

		for _, use := range ti.Uses {
			resolveUses(tis, ti, use)
			tis[i] = ti
		}
	}

	return tis, nil
}

func findTerminfo(tis []*Terminfo, name string) *Terminfo {
	for _, ti := range tis {
		for _, n := range ti.Names {
			if n == name {
				return ti
			}
		}
	}
	return nil
}

func resolveUses(tis []*Terminfo, ti *Terminfo, use string) {
	u := findTerminfo(tis, use)
	if u == nil {
		log.Printf("Warn: %q uses %q, but %q is not found", ti.Names[0], use, use)
		return
	}

	for _, uu := range u.Uses {
		resolveUses(tis, ti, uu)
	}

	// XXX: We need to check if the caps already exist in the terminfo
	// so we don't override them.

	// Collect core caps
	for k, v := range u.Bools {
		if _, ok := ti.Bools[k]; !ok {
			ti.Bools[k] = v
		}
	}
	for k, v := range u.Nums {
		if _, ok := ti.Nums[k]; !ok {
			ti.Nums[k] = v
		}
	}
	for k, v := range u.Strings {
		if _, ok := ti.Strings[k]; !ok {
			ti.Strings[k] = v
		}
	}

	// Collect extended caps
	for k, v := range u.ExtBoolNames {
		if _, ok := ti.ExtBoolNames[k]; !ok {
			ti.ExtBoolNames[k] = v
		}
	}

	for k, v := range u.ExtNumNames {
		if _, ok := ti.ExtNumNames[k]; !ok {
			ti.ExtNumNames[k] = v
		}
	}

	for k, v := range u.ExtStringNames {
		if _, ok := ti.ExtStringNames[k]; !ok {
			ti.ExtStringNames[k] = v
		}
	}

	for k, v := range u.ExtBools {
		if _, ok := ti.ExtBools[k]; !ok {
			ti.ExtBools[k] = v
		}
	}

	for k, v := range u.ExtNums {
		if _, ok := ti.ExtNums[k]; !ok {
			ti.ExtNums[k] = v
		}
	}

	for k, v := range u.ExtStrings {
		if _, ok := ti.ExtStrings[k]; !ok {
			ti.ExtStrings[k] = v
		}
	}
}

const (
	GROUND = iota
	INT
	NONE
	CTRL
	ESC
)
