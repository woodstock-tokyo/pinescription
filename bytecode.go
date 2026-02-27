// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
)

const bytecodeMagic = "PG2\x00"

func encodeProgram(program Program) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(bytecodeMagic)
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(program); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeProgram(bytecode []byte) (Program, error) {
	var program Program
	var err error
	if bytes.HasPrefix(bytecode, []byte(bytecodeMagic)) {
		dec := gob.NewDecoder(bytes.NewReader(bytecode[len(bytecodeMagic):]))
		err = dec.Decode(&program)
	} else {
		err = json.Unmarshal(bytecode, &program)
	}
	if program.Functions == nil {
		program.Functions = map[string]FunctionDef{}
	}
	if program.Types == nil {
		program.Types = map[string]TypeDef{}
	}
	lowerProgram(&program)
	return program, err
}
