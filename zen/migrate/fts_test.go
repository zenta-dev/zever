package migrate

import (
	"testing"
)

func TestCreateFTS5VirtualTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		table   string
		columns []string
		want    string
		wantErr bool
	}{
		{
			name:    "singleColumn",
			table:   "docs",
			columns: []string{"body"},
			want:    `CREATE VIRTUAL TABLE IF NOT EXISTS "docs" USING fts5(body);`,
		},
		{
			name:    "multiColumn",
			table:   "docs",
			columns: []string{"title", "body"},
			want:    `CREATE VIRTUAL TABLE IF NOT EXISTS "docs" USING fts5(title, body);`,
		},
		{"badTable", "drop table", []string{"body"}, "", true},
		{"emptyTable", "", []string{"body"}, "", true},
		{"noColumns", "docs", nil, "", true},
		{"badColumn", "docs", []string{"ok", "drop it"}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := CreateFTS5VirtualTable(tt.table, tt.columns)
			if tt.wantErr {
				if err == nil {
					t.Fatal("CreateFTS5VirtualTable: want error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("CreateFTS5VirtualTable: %v", err)
			}

			if got != tt.want {
				t.Fatalf("CreateFTS5VirtualTable = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDropFTS5VirtualTable(t *testing.T) {
	t.Parallel()

	got := DropFTS5VirtualTable("docs")
	want := `DROP TABLE IF EXISTS "docs";`

	if got != want {
		t.Fatalf("DropFTS5VirtualTable = %q, want %q", got, want)
	}
}
