package device

import (
	"fmt"
	"hash/fnv"
)

func redactIdentifier(value string) string {
	if value == "" {
		return "unknown"
	}
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(value))
	return fmt.Sprintf("id:%08x", hasher.Sum32())
}
