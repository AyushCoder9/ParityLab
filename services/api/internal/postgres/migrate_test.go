package postgres

import "testing"

func TestStripTransactionWrapper(t *testing.T) {
	t.Parallel()
	got, err := stripTransactionWrapper("\nBEGIN;\nCREATE TABLE example (id int);\nCOMMIT;\n")
	if err != nil {
		t.Fatal(err)
	}
	if got != "CREATE TABLE example (id int);" {
		t.Fatalf("unexpected SQL: %q", got)
	}
	if _, err := stripTransactionWrapper("CREATE TABLE unsafe (id int);"); err == nil {
		t.Fatal("accepted migration without transaction wrapper")
	}
}
