package main

import "encoding/json"

func jsonify(v any) []byte {
	output, err := json.Marshal(v)
	if err != nil {
		panic("unable to jsonify: " + err.Error())
	}
	return output
}