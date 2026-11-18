package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

func main() {
	outDir := filepath.Join("cmd", "corechain")
	_ = os.MkdirAll(outDir, 0755)

	png32 := renderCoreChainPNG(32)
	png64 := renderCoreChainPNG(64)
	png256 := renderCoreChainPNG(256)

	ico := buildICO([][]byte{png32, png64, png256}, []int{32, 64, 256})
	_ = os.WriteFile(filepath.Join(outDir, "corechain.ico"), ico, 0644)

	// Build .syso for both arm64 and amd64
	if err := writeSyso(filepath.Join(outDir, "rsrc_windows_arm64.syso"), 0xAA64, 0x0002, [][]byte{png32, png64, png256}, []int{32, 64, 256}); err != nil {
		panic(err)
	}
	if err := writeSyso(filepath.Join(outDir, "rsrc_windows_amd64.syso"), 0x8664, 0x0003, [][]byte{png32, png64, png256}, []int{32, 64, 256}); err != nil {
		panic(err)
	}
	fmt.Println("Generated corechain.ico and Windows .syso icon resources!")
}

func renderCoreChainPNG(size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	cx, cy := s/2.0, s/2.0
	rad := s * 0.46
	corner := s * 0.24

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			// Rounded square SDF
			dx := math.Max(math.Abs(fx-cx)-(rad-corner), 0)
			dy := math.Max(math.Abs(fy-cy)-(rad-corner), 0)
			distBox := math.Hypot(dx, dy) - corner
			if distBox > 0.8 {
				continue
			}
			alpha := clamp01(0.8 - distBox)

			// Sky-blue to Royal-blue gradient
			t := clamp01((fx + fy) / (2.0 * s))
			r := lerp(14, 29, t)
			g := lerp(165, 78, t)
			b := lerp(233, 216, t)

			// Subtle bright border
			if distBox > -s*0.045 {
				r = lerp(r, 125, 0.55)
				g = lerp(g, 211, 0.55)
				b = lerp(b, 252, 0.55)
			}

			// Dark slate inner circle
			distCenter := math.Hypot(fx-cx, fy-cy)
			innerR := s * 0.34
			if distCenter < innerR {
				inBlend := clamp01(innerR - distCenter)
				r = lerp(r, 15, inBlend)
				g = lerp(g, 23, inBlend)
				b = lerp(b, 42, inBlend)
			}

			// Interlocking Chain Link 1 (top-left, white)
			d1 := capsuleRingDist(fx, fy, cx-s*0.09, cy-s*0.08, s*0.14, s*0.09, -0.55)
			if d1 < s*0.042 {
				w := clamp01((s*0.042 - d1) * 1.5)
				r, g, b = lerp(r, 255, w), lerp(g, 255, w), lerp(b, 255, w)
			}

			// Interlocking Chain Link 2 (bottom-right, cyan #38bdf8)
			d2 := capsuleRingDist(fx, fy, cx+s*0.09, cy+s*0.08, s*0.14, s*0.09, -0.55)
			if d2 < s*0.042 {
				w := clamp01((s*0.042 - d2) * 1.5)
				r, g, b = lerp(r, 56, w), lerp(g, 189, w), lerp(b, 248, w)
			}

			// Glowing center core dot
			if distCenter < s*0.06 {
				w := clamp01((s*0.06 - distCenter) * 1.5)
				r, g, b = lerp(r, 250, w), lerp(g, 204, w), lerp(b, 21, w)
			}

			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r),
				G: uint8(g),
				B: uint8(b),
				A: uint8(alpha * 255),
			})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func capsuleRingDist(px, py, cx, cy, rx, ry, angle float64) float64 {
	dx, dy := px-cx, py-cy
	ca, sa := math.Cos(angle), math.Sin(angle)
	lx := dx*ca - dy*sa
	ly := dx*sa + dy*ca
	// Rounded rectangle ring distance
	qx := math.Max(math.Abs(lx)-rx+ry, 0)
	qy := math.Abs(ly)
	d := math.Hypot(qx, qy) - ry
	return math.Abs(d)
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func buildICO(pngs [][]byte, sizes []int) []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0)) // reserved
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // type 1 = ICO
	_ = binary.Write(&buf, binary.LittleEndian, uint16(len(pngs)))

	offset := uint32(6 + 16*len(pngs))
	for i, p := range pngs {
		sz := byte(sizes[i])
		if sizes[i] >= 256 {
			sz = 0
		}
		buf.WriteByte(sz)
		buf.WriteByte(sz)
		buf.WriteByte(0)
		buf.WriteByte(0)
		_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
		_ = binary.Write(&buf, binary.LittleEndian, uint16(32))
		_ = binary.Write(&buf, binary.LittleEndian, uint32(len(p)))
		_ = binary.Write(&buf, binary.LittleEndian, offset)
		offset += uint32(len(p))
	}
	for _, p := range pngs {
		buf.Write(p)
	}
	return buf.Bytes()
}

// writeSyso writes a minimal COFF object file containing .rsrc with RT_ICON + RT_GROUP_ICON.
func writeSyso(outPath string, machine uint16, relocType uint16, pngs [][]byte, sizes []int) error {
	// Build GRPICONDIR blob
	var grp bytes.Buffer
	_ = binary.Write(&grp, binary.LittleEndian, uint16(0))
	_ = binary.Write(&grp, binary.LittleEndian, uint16(1))
	_ = binary.Write(&grp, binary.LittleEndian, uint16(len(pngs)))
	for i, p := range pngs {
		sz := byte(sizes[i])
		if sizes[i] >= 256 {
			sz = 0
		}
		grp.WriteByte(sz)
		grp.WriteByte(sz)
		grp.WriteByte(0)
		grp.WriteByte(0)
		_ = binary.Write(&grp, binary.LittleEndian, uint16(1))
		_ = binary.Write(&grp, binary.LittleEndian, uint16(32))
		_ = binary.Write(&grp, binary.LittleEndian, uint32(len(p)))
		_ = binary.Write(&grp, binary.LittleEndian, uint16(i+1)) // icon resource ID
	}

	// Layout .rsrc section:
	// 1. Root dir (16 + 2*8 = 32 bytes): Type 3 (RT_ICON) and Type 14 (RT_GROUP_ICON)
	// 2. Type 3 dir (16 + N*8 bytes) -> N icon IDs (1..N)
	// 3. Type 14 dir (16 + 1*8 = 24 bytes) -> ID 1
	// 4. Lang dirs: (N + 1) * (16 + 8 = 24 bytes)
	// 5. Data entries: (N + 1) * 16 bytes
	// 6. Raw data blobs (padded to 4 bytes)
	n := len(pngs)
	rootOff := uint32(0)
	type3Off := uint32(32)
	type14Off := type3Off + uint32(16+n*8)
	langOff := type14Off + uint32(24)
	dataEntryOff := langOff + uint32((n+1)*24)
	blobOff := dataEntryOff + uint32((n+1)*16)

	var sec bytes.Buffer
	writeDirHeader := func(nIDs uint16) {
		_ = binary.Write(&sec, binary.LittleEndian, uint32(0)) // Characteristics
		_ = binary.Write(&sec, binary.LittleEndian, uint32(0)) // TimeDateStamp
		_ = binary.Write(&sec, binary.LittleEndian, uint16(0)) // Major
		_ = binary.Write(&sec, binary.LittleEndian, uint16(0)) // Minor
		_ = binary.Write(&sec, binary.LittleEndian, uint16(0)) // NumberOfNamedEntries
		_ = binary.Write(&sec, binary.LittleEndian, nIDs)      // NumberOfIdEntries
	}
	writeDirEntry := func(id uint32, offset uint32, isDir bool) {
		_ = binary.Write(&sec, binary.LittleEndian, id)
		val := offset
		if isDir {
			val |= 0x80000000
		}
		_ = binary.Write(&sec, binary.LittleEndian, val)
	}

	// Root
	_ = rootOff
	writeDirHeader(2)
	writeDirEntry(3, type3Off, true)   // RT_ICON
	writeDirEntry(14, type14Off, true) // RT_GROUP_ICON

	// Type 3 (RT_ICON)
	writeDirHeader(uint16(n))
	for i := 0; i < n; i++ {
		writeDirEntry(uint32(i+1), langOff+uint32(i*24), true)
	}

	// Type 14 (RT_GROUP_ICON)
	writeDirHeader(1)
	writeDirEntry(1, langOff+uint32(n*24), true)

	// Lang dirs
	for i := 0; i < n+1; i++ {
		writeDirHeader(1)
		writeDirEntry(0x0409, dataEntryOff+uint32(i*16), false)
	}

	// Data entries + record relocation offsets
	type reloc struct {
		vaddr uint32
	}
	var relocs []reloc
	blobs := append([][]byte{}, pngs...)
	blobs = append(blobs, grp.Bytes())

	curBlob := blobOff
	for i, b := range blobs {
		relocs = append(relocs, reloc{vaddr: dataEntryOff + uint32(i*16)})
		_ = binary.Write(&sec, binary.LittleEndian, curBlob)        // OffsetToData (relocated!)
		_ = binary.Write(&sec, binary.LittleEndian, uint32(len(b))) // Size
		_ = binary.Write(&sec, binary.LittleEndian, uint32(0))      // CodePage
		_ = binary.Write(&sec, binary.LittleEndian, uint32(0))      // Reserved
		padded := uint32((len(b) + 3) &^ 3)
		curBlob += padded
	}

	// Blobs
	for _, b := range blobs {
		sec.Write(b)
		for sec.Len()%4 != 0 {
			sec.WriteByte(0)
		}
	}

	// Assemble COFF file
	// Header (20) + SectionHeader (40) = 60
	rawDataOffset := uint32(60)
	relocOffset := rawDataOffset + uint32(sec.Len())
	symTableOffset := relocOffset + uint32(len(relocs)*10)

	var coff bytes.Buffer
	// IMAGE_FILE_HEADER
	_ = binary.Write(&coff, binary.LittleEndian, machine)
	_ = binary.Write(&coff, binary.LittleEndian, uint16(1))             // NumberOfSections
	_ = binary.Write(&coff, binary.LittleEndian, uint32(0))             // TimeDateStamp
	_ = binary.Write(&coff, binary.LittleEndian, symTableOffset)        // PointerToSymbolTable
	_ = binary.Write(&coff, binary.LittleEndian, uint32(1))             // NumberOfSymbols
	_ = binary.Write(&coff, binary.LittleEndian, uint16(0))             // SizeOfOptionalHeader
	_ = binary.Write(&coff, binary.LittleEndian, uint16(0x0100))        // Characteristics

	// IMAGE_SECTION_HEADER (.rsrc)
	var secName [8]byte
	copy(secName[:], ".rsrc")
	coff.Write(secName[:])
	_ = binary.Write(&coff, binary.LittleEndian, uint32(0))             // VirtualSize
	_ = binary.Write(&coff, binary.LittleEndian, uint32(0))             // VirtualAddress
	_ = binary.Write(&coff, binary.LittleEndian, uint32(sec.Len()))     // SizeOfRawData
	_ = binary.Write(&coff, binary.LittleEndian, rawDataOffset)         // PointerToRawData
	_ = binary.Write(&coff, binary.LittleEndian, relocOffset)           // PointerToRelocations
	_ = binary.Write(&coff, binary.LittleEndian, uint32(0))             // PointerToLinenumbers
	_ = binary.Write(&coff, binary.LittleEndian, uint16(len(relocs)))   // NumberOfRelocations
	_ = binary.Write(&coff, binary.LittleEndian, uint16(0))             // NumberOfLinenumbers
	_ = binary.Write(&coff, binary.LittleEndian, uint32(0x40000040))    // Characteristics

	// Raw section data
	coff.Write(sec.Bytes())

	// Relocations (each 10 bytes, pointing to symbol 0 = .rsrc)
	for _, r := range relocs {
		_ = binary.Write(&coff, binary.LittleEndian, r.vaddr)
		_ = binary.Write(&coff, binary.LittleEndian, uint32(0)) // SymbolTableIndex = 0
		_ = binary.Write(&coff, binary.LittleEndian, relocType)
	}

	// Symbol 0: ".rsrc" (18 bytes)
	coff.Write(secName[:])
	_ = binary.Write(&coff, binary.LittleEndian, uint32(0)) // Value
	_ = binary.Write(&coff, binary.LittleEndian, int16(1))  // SectionNumber = 1
	_ = binary.Write(&coff, binary.LittleEndian, uint16(0)) // Type
	coff.WriteByte(3)                                       // StorageClass = IMAGE_SYM_CLASS_STATIC
	coff.WriteByte(0)                                       // NumberOfAuxSymbols

	// String table size (4 bytes)
	_ = binary.Write(&coff, binary.LittleEndian, uint32(4))

	return os.WriteFile(outPath, coff.Bytes(), 0644)
}
