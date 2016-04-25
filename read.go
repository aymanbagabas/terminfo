package terminfo

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"
)

var (
	ErrBadMagic  = errors.New("terminfo: wrong filetype for terminfo file")
	ErrSmallFile = errors.New("terminfo: file too small")
	ErrBadString = errors.New("terminfo: bad string")
)

// header represents a Terminfo file's header.
type header [6]int16

// badMagic returns false if the correct magic number is set on the header and true otherwise.
func (h header) badMagic() bool {
	if h[0] == 0x11A {
		return false
	}
	return true
}

// lenNames returns the length of name section
func (h header) lenNames() int16 {
	return h[1]
}

// lenBools returns the length of boolean section
func (h header) lenBools() int16 {
	return h[2]
}

// lenNumeric returns the length of numeric section
func (h header) lenNumeric() int16 {
	return h[3] * 2 // stored as number of int16
}

// lenStrings returns the length of string section
func (h header) lenStrings() int16 {
	return h[4] * 2 // stored as number of int16
}

// lenTable returns the length of string table in bytes.
func (h header) lenTable() int16 {
	return h[5]
}

// lenFile returns the length of the file the header describes.
func (h header) lenFile() int16 {
	return h[1] + h[2] + h[3] + h[4] + h[5]
}

// len returns the length of the header in bytes.
func (h header) len() int16 {
	return int16(len(h) * 2)
}

// skipNull returns true if an extra null byte was added to align everything
// on word boundaries and false otherwise.
func (h header) skipNull() bool {
	return (h.lenNames()+h.lenBools())%2 == 1
}

// TODO extended reader
type reader struct {
	pos, ppos int16
	buf       []byte
	ti        *Terminfo
	h         header
}

var readerPool = sync.Pool{
	New: func() interface{} {
		r := new(reader)
		r.buf = make([]byte, 3000)
		return r
	},
}

func (r *reader) free() {
	r.pos, r.ppos = 0, 0
	r.h = header{}
	readerPool.Put(r)
}

func (r *reader) slice() []byte {
	return r.buf[r.ppos:r.pos]
}

func (r *reader) sliceOff(off int16) []byte {
	r.ppos, r.pos = r.pos, r.pos+off
	return r.slice()
}

func (r *reader) read(f *os.File) error {
	if err := r.readHeader(f); err != nil {
		return err
	}
	r.readNames()
	r.readBools()
	r.readNumbers()
	return r.readStrings()
}

func (r *reader) readHeader(f *os.File) error {
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	s := fi.Size()
	if s < int64(len(r.h)) {
		return ErrSmallFile
	}
	if s < int64(len(r.buf)) {
		r.buf = make([]byte, s)
	}
	if _, err = io.ReadFull(f, r.buf); err != nil {
		return err
	}
	for i := 0; i < len(r.h); i++ {
		r.h[i] = littleEndian(i*2, r.buf)
	}
	if int(r.h.lenFile()) > len(r.buf) {
		return ErrSmallFile
	}
	if r.h.badMagic() {
		return ErrBadMagic
	}
	return nil
}

func (r *reader) readNames() {
	r.ppos = r.h.len()
	r.pos = r.ppos + r.h.lenNames()
	r.ti = new(Terminfo)
	r.ti.Names = strings.Split(string(r.slice()), "|")
}

func (r *reader) readBools() {
	for i, b := range r.sliceOff(r.h.lenBools()) {
		if b == 1 {
			r.ti.BoolCaps[i] = true
		}
	}
	if r.h.skipNull() {
		// Skip extra null byte inserted to align everything on word boundaries.
		r.pos++
	}
}

func (r *reader) readNumbers() {
	nbuf := r.sliceOff(r.h.lenNumeric())
	for j := 0; j < len(nbuf); j += 2 {
		if n := littleEndian(j, nbuf); n > -1 {
			r.ti.NumericCaps[j/2] = n
		}
	}
}

func (r *reader) readStrings() error {
	// Read the string and string table section.
	sbuf := r.sliceOff(r.h.lenStrings())
	table := r.buf[r.pos : r.pos+r.h.lenTable()]
	for j := 0; j < len(sbuf); j += 2 {
		if off := littleEndian(j, sbuf); off > -1 {
			x := int(off)
			for ; table[x] != 0; x++ {
				if x+1 >= len(table) {
					return ErrBadString
				}
			}
			r.ti.StringCaps[j/2] = string(table[off:x])
		}
	}
	return nil
}