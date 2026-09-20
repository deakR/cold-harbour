package main

import (
	"encoding/json"
	"fmt"
)

func main() {
	const fixture = `Contact jane.doe@example.com or (555) 123-4567 for details.
SSN on file: 123-45-6789. Backup contact: john@company.org.
Not a match: version 123-45 or year 1234-56-789.`

	result, err := RedactPII(fixture)
	if err != nil {
		panic(err)
	}
	b, err := json.Marshal(result)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
}
