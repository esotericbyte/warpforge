package wfapi

import (
	"github.com/ipld/go-ipld-prime/schema"
)

func init() {
	TypeSystem.Accumulate(schema.SpawnStruct("WareID",
		[]schema.StructField{
			schema.SpawnStructField("packtype", "String", false, false),
			schema.SpawnStructField("hash", "String", false, false),
		},
		schema.SpawnStructRepresentationStringjoin(":")))
}

type WareID struct {
	PackType string // f.eks. "tar", "git"
	Hash     string // what it says on the tin.
}