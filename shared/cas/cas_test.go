package cas_test

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/shared/cas"
)

func TestScripts_atomicOwnerCheck(t *testing.T) {
	t.Parallel()

	for name, script := range map[string]string{
		"delete":  cas.CompareAndDeleteScript,
		"expire":  cas.CompareAndExpireScript,
		"persist": cas.CompareAndPersistScript,
	} {
		if !strings.Contains(script, `redis.call("GET", KEYS[1]) == ARGV[1]`) {
			t.Errorf("%s script missing owner check", name)
		}
	}

	if !strings.Contains(cas.CompareAndDeleteScript, `redis.call("DEL"`) {
		t.Error("delete script missing DEL")
	}

	if !strings.Contains(cas.CompareAndExpireScript, `redis.call("PEXPIRE"`) {
		t.Error("expire script missing PEXPIRE")
	}

	if !strings.Contains(cas.CompareAndPersistScript, `redis.call("PERSIST"`) {
		t.Error("persist script missing PERSIST")
	}
}
