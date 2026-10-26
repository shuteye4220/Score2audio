// Command makefat joins Mac programs built for different processors
// (Apple silicon and Intel) into one universal program.
//
//	go run ./tools/makefat OUTPUT INPUT...
package main

import (
	"debug/macho"
	"encoding/binary"
	"fmt"
	"os"
)

const align = 14 // 16 KiB, as Apple's tools use

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: makefat OUTPUT INPUT INPUT...")
		os.Exit(2)
	}
	type input struct {
		data        []byte
		cpu, subCPU uint32
		offset      uint32
	}
	var ins []input
	offset := uint32(1 << align)
	for _, name := range os.Args[2:] {
		data, err := os.ReadFile(name)
		check(err)
		f, err := macho.Open(name)
		check(err)
		ins = append(ins, input{data: data, cpu: uint32(f.Cpu), subCPU: f.SubCpu, offset: offset})
		f.Close()
		offset += uint32(len(data))
		offset = (offset + 1<<align - 1) &^ (1<<align - 1)
	}
	out := make([]byte, offset)
	be := binary.BigEndian
	be.PutUint32(out[0:], macho.MagicFat)
	be.PutUint32(out[4:], uint32(len(ins)))
	for i, in := range ins {
		h := out[8+20*i:]
		be.PutUint32(h[0:], in.cpu)
		be.PutUint32(h[4:], in.subCPU)
		be.PutUint32(h[8:], in.offset)
		be.PutUint32(h[12:], uint32(len(in.data)))
		be.PutUint32(h[16:], align)
		copy(out[in.offset:], in.data)
	}
	check(os.WriteFile(os.Args[1], out[:ins[len(ins)-1].offset+uint32(len(ins[len(ins)-1].data))], 0o755))
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "makefat:", err)
		os.Exit(1)
	}
}
