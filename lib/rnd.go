package lib

import (
	"encoding/hex"
	"math/rand"
)

func GenRandomByteArray(r *rand.Rand, length uint64) []byte {
	newHeaderBytes := make([]byte, length)
	r.Read(newHeaderBytes)
	return newHeaderBytes
}

func GenRandomHexStr(r *rand.Rand, length uint64) string {
	randBytes := GenRandomByteArray(r, length)
	return hex.EncodeToString(randBytes)
}
