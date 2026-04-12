package persona

import "testing"

func TestAllNames(t *testing.T) {
	got := AllNames()
	want := []Name{NameSoul, NameUser, NameMemory}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestIsValid(t *testing.T) {
	valid := []string{"soul", "user", "memory"}
	for _, v := range valid {
		if !IsValid(v) {
			t.Errorf("IsValid(%q) = false, want true", v)
		}
	}
	invalid := []string{"", "SOUL", "User", "bogus", "profile"}
	for _, v := range invalid {
		if IsValid(v) {
			t.Errorf("IsValid(%q) = true, want false", v)
		}
	}
}

func TestNameFile(t *testing.T) {
	tests := []struct {
		name Name
		want string
	}{
		{NameSoul, "SOUL.md"},
		{NameUser, "USER.md"},
		{NameMemory, "MEMORY.md"},
		{Name("unknown"), ""},
	}
	for _, tt := range tests {
		if got := tt.name.File(); got != tt.want {
			t.Errorf("%q.File() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestNameBudget(t *testing.T) {
	tests := []struct {
		name Name
		want int
	}{
		{NameSoul, 3500},
		{NameUser, 2000},
		{NameMemory, 2500},
		{Name("unknown"), 0},
	}
	for _, tt := range tests {
		if got := tt.name.Budget(); got != tt.want {
			t.Errorf("%q.Budget() = %d, want %d", tt.name, got, tt.want)
		}
	}
}
