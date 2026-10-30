package github

import "encoding/json"

// jsonRaw holds one part of a response, decoded later, for a query whose
// fields are named at run time.
type jsonRaw = json.RawMessage

func decode(raw jsonRaw, v any) error { return json.Unmarshal(raw, v) }
