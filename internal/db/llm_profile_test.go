package db

import "testing"

func mkProfile(name, defModel string, models ...string) *LLMProfile {
	return &LLMProfile{
		Name:         name,
		BaseURL:      "https://api.example.com/v1",
		Models:       models,
		DefaultModel: defModel,
		Temperature:  0.2,
	}
}

func TestLLMProfileCRUD(t *testing.T) {
	d := testDB(t)

	p := mkProfile("DeepSeek", "deepseek-chat", "deepseek-chat", "deepseek-reasoner")
	if err := d.CreateLLMProfile(p); err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("expected non-zero id")
	}

	got, err := d.GetLLMProfile(p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "DeepSeek" || got.DefaultModel != "deepseek-chat" {
		t.Errorf("unexpected profile: %+v", got)
	}
	if len(got.Models) != 2 || got.Models[0] != "deepseek-chat" {
		t.Errorf("models = %v", got.Models)
	}
	if got.Temperature != 0.2 {
		t.Errorf("temperature = %v", got.Temperature)
	}
	if got.IsActive {
		t.Error("new profile should not be active by default")
	}

	// Update.
	got.Name = "DeepSeek Prod"
	got.Models = JSONStringSlice{"deepseek-reasoner"}
	got.DefaultModel = "deepseek-reasoner"
	got.Temperature = 0.7
	if err := d.UpdateLLMProfile(got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, _ := d.GetLLMProfile(p.ID)
	if got2.Name != "DeepSeek Prod" || got2.Temperature != 0.7 || len(got2.Models) != 1 {
		t.Errorf("update not applied: %+v", got2)
	}

	// List.
	list, err := d.ListLLMProfiles()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(list))
	}

	// Delete.
	if err := d.DeleteLLMProfile(p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = d.ListLLMProfiles()
	if len(list) != 0 {
		t.Errorf("expected 0 profiles after delete, got %d", len(list))
	}
}

func TestSetActiveLLMProfile_Single(t *testing.T) {
	d := testDB(t)

	a := mkProfile("A", "m1", "m1")
	b := mkProfile("B", "m2", "m2")
	c := mkProfile("C", "m3", "m3")
	for _, p := range []*LLMProfile{a, b, c} {
		if err := d.CreateLLMProfile(p); err != nil {
			t.Fatalf("create %s: %v", p.Name, err)
		}
	}

	// No active profile yet.
	act, err := d.GetActiveLLMProfile()
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if act != nil {
		t.Fatalf("expected no active profile, got %+v", act)
	}

	if err := d.SetActiveLLMProfile(a.ID); err != nil {
		t.Fatalf("set active a: %v", err)
	}
	act, _ = d.GetActiveLLMProfile()
	if act == nil || act.ID != a.ID {
		t.Fatalf("expected A active, got %+v", act)
	}

	// Switching active must leave exactly one active.
	if err := d.SetActiveLLMProfile(b.ID); err != nil {
		t.Fatalf("set active b: %v", err)
	}
	if err := d.SetActiveLLMProfile(c.ID); err != nil {
		t.Fatalf("set active c: %v", err)
	}

	list, _ := d.ListLLMProfiles()
	activeCount := 0
	for _, p := range list {
		if p.IsActive {
			activeCount++
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly 1 active profile, got %d", activeCount)
	}
	act, _ = d.GetActiveLLMProfile()
	if act.ID != c.ID {
		t.Errorf("expected C active, got %+v", act)
	}
}

func TestSetActiveLLMProfile_NotFound(t *testing.T) {
	d := testDB(t)
	if err := d.SetActiveLLMProfile(999); err == nil {
		t.Error("expected error activating non-existent profile")
	}
}

func TestLLMProfile_UniqueName(t *testing.T) {
	d := testDB(t)
	if err := d.CreateLLMProfile(mkProfile("Dup", "m", "m")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := d.CreateLLMProfile(mkProfile("Dup", "m", "m")); err == nil {
		t.Error("expected UNIQUE(name) violation")
	}
}
