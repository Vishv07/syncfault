package scenario

import (
	"fmt"
	"path/filepath"
	"strings"
)

func init() {
	RegisterRecipe("unicode_norm", expandUnicodeNorm)
}

var unicodePairs = []struct {
	nfc string
	nfd string
}{
	{nfc: "caf\u00e9", nfd: "cafe\u0301"},
	{nfc: "r\u00e9sum\u00e9", nfd: "re\u0301sume\u0301"},
	{nfc: "na\u00efve", nfd: "nai\u0308ve"},
	{nfc: "pi\u00f1ata", nfd: "pin\u0303ata"},
}

var nfdToNFCReplacer = strings.NewReplacer(
	"a\u0300", "\u00e0",
	"a\u0301", "\u00e1",
	"a\u0308", "\u00e4",
	"a\u030a", "\u00e5",
	"c\u0327", "\u00e7",
	"e\u0300", "\u00e8",
	"e\u0301", "\u00e9",
	"e\u0302", "\u00ea",
	"e\u0308", "\u00eb",
	"i\u0301", "\u00ed",
	"i\u0308", "\u00ef",
	"n\u0303", "\u00f1",
	"o\u0301", "\u00f3",
	"o\u0308", "\u00f6",
	"u\u0301", "\u00fa",
	"u\u0308", "\u00fc",
	"A\u0301", "\u00c1",
	"E\u0301", "\u00c9",
	"N\u0303", "\u00d1",
	"O\u0308", "\u00d6",
	"U\u0308", "\u00dc",
)

// NormalizeNFC converts decomposed UTF-8 combining sequences (NFD) into composed NFC form.
func NormalizeNFC(s string) string {
	return nfdToNFCReplacer.Replace(s)
}

// expandUnicodeNorm generates files using NFC composed and NFD decomposed UTF-8 filenames
// across replicas to test cross-platform macOS APFS (NFD) vs Linux/Cloud (NFC) normalization.
func expandUnicodeNorm(spec WorkloadSpec, stream *Stream) ([]PlannedOp, error) {
	side := primarySide(spec)
	fileCount := spec.Files
	if fileCount <= 0 {
		fileCount = 6
	}

	var ops []PlannedOp
	var nfcPaths []string

	// Phase 1: Create NFC-named files on primary side and run baseline sync
	for i := 0; i < fileCount; i++ {
		pair := unicodePairs[i%len(unicodePairs)]
		rel := filepath.ToSlash(fmt.Sprintf("intl/%s_%03d.txt", pair.nfc, i+1))
		size := stream.Int64Range(spec.FileSize.MinBytes, spec.FileSize.MaxBytes)
		ops = append(ops, PlannedOp{
			Side: side,
			Kind: OpCreate,
			Path: rel,
			Size: size,
		})
		nfcPaths = append(nfcPaths, rel)
	}
	ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})

	// Phase 2: Edit half of the files using their NFD decomposed path representation
	// and half using their NFC representation to exercise normalization equivalence.
	for i, nfcRel := range nfcPaths {
		pair := unicodePairs[i%len(unicodePairs)]
		targetRel := nfcRel
		if i%2 == 0 {
			targetRel = filepath.ToSlash(fmt.Sprintf("intl/%s_%03d.txt", pair.nfd, i+1))
		}
		size := stream.Int64Range(spec.FileSize.MinBytes, spec.FileSize.MaxBytes)
		ops = append(ops, PlannedOp{
			Side: side,
			Kind: OpEdit,
			Path: targetRel,
			Size: size,
		})
		if (i+1)%2 == 0 {
			ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})
		}
	}
	ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})

	return ops, nil
}
