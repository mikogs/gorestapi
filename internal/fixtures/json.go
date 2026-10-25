package fixtures

import (
	"encoding/json"
	"fmt"
	"time"
)

func TestStructJSON() []byte {
	data := map[string]any{
		"Flags":          4,
		"PrimaryEmail":   "primary@example.com",
		"EmailSecondary": "secondary@example.com",
		"FirstName":      "John",
		"LastName":       "Smith",
		"Age":            37,
		"Price":          444,
		"PostCode":       "00-000",
		"PostCode2":      "11-111",
		"Password":       "yyy",
		"Key":            fmt.Sprintf("12345679012345678901234567890%d", time.Now().UnixNano()),
	}
	jsonBytes, _ := json.Marshal(data)
	return jsonBytes
}
