package scenario

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math/rand/v2"
)

const pcgSalt uint64 = 0x9e3779b97f4a7c15

// Stream provides a single deterministic PCG random stream derived strictly from a run seed.
type Stream struct {
	seed uint64
	rng  *rand.Rand
}

// NewStream constructs a deterministic PCG stream for the given run seed.
func NewStream(seed uint64) *Stream {
	src := rand.NewPCG(seed, seed^pcgSalt)
	return &Stream{
		seed: seed,
		rng:  rand.New(src),
	}
}

// Seed returns the base seed of the stream.
func (s *Stream) Seed() uint64 {
	return s.seed
}

// IntRange returns a deterministic integer in [minVal, maxVal].
func (s *Stream) IntRange(minVal, maxVal int) int {
	if maxVal <= minVal {
		return minVal
	}
	return minVal + s.rng.IntN(maxVal-minVal+1)
}

// Int64Range returns a deterministic int64 in [minVal, maxVal].
func (s *Stream) Int64Range(minVal, maxVal int64) int64 {
	if maxVal <= minVal {
		return minVal
	}
	return minVal + s.rng.Int64N(maxVal-minVal+1)
}

// JitterMS computes the deterministic jitter in milliseconds for a FaultTrigger.
func (s *Stream) JitterMS(j JitterSpec) int {
	if !j.Seeded || j.MaxMS <= j.MinMS {
		return j.MinMS
	}
	return s.IntRange(j.MinMS, j.MaxMS)
}

// ContentBytes deterministically generates size bytes for (seed, opIndex) so file contents
// are 100% reproducible without keeping copies in memory.
func ContentBytes(seed uint64, opIndex int, size int64) []byte {
	if size <= 0 {
		return nil
	}
	src := rand.NewPCG(seed^uint64(opIndex*1315423911), (seed+uint64(opIndex))^pcgSalt)
	r := rand.New(src)
	buf := make([]byte, size)
	var i int64
	for i+8 <= size {
		binary.LittleEndian.PutUint64(buf[i:i+8], r.Uint64())
		i += 8
	}
	if i < size {
		rem := r.Uint64()
		for ; i < size; i++ {
			buf[i] = byte(rem & 0xff)
			rem >>= 8
		}
	}
	return buf
}

// HashBytes returns the hex-encoded SHA-256 digest of data.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
