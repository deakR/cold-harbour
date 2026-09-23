package migrations

import "embed"

//go:embed *.sql
var FS embed.FS

func Files() embed.FS {
	return FS
}
