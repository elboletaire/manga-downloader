// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"testing"
)

func TestParseSetup(t *testing.T) {
	wait, result, err := parseSetup(`{"wait":true,"result":{"selected":"English"}}`)
	if err != nil || !wait || string(result) != `{"selected":"English"}` {
		t.Errorf("parseSetup = %v, %s, %v; want wait and the result", wait, result, err)
	}

	wait, _, err = parseSetup(`{"wait":false,"result":null}`)
	if err != nil || wait {
		t.Errorf("parseSetup without a request = %v, %v; want no wait", wait, err)
	}

	if _, _, err := parseSetup(""); err == nil {
		t.Error("parseSetup of an empty result should fail")
	}
}
