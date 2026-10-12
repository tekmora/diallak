package serverdef

import (
	"strings"
	"testing"
)

func swapRoles(order []string, a, b string) {
	ia, ib := -1, -1
	for i, r := range order {
		switch r {
		case a:
			ia = i
		case b:
			ib = i
		}
	}
	if ia >= 0 && ib >= 0 {
		order[ia], order[ib] = order[ib], order[ia]
	}
}

func moveAfter(order []string, target, pivot string) []string {
	res := make([]string, 0, len(order))
	for _, r := range order {
		if r == target {
			continue
		}
		res = append(res, r)
		if r == pivot {
			res = append(res, target)
		}
	}
	return res
}

func moveBefore(order []string, target, pivot string) []string {
	res := make([]string, 0, len(order))
	for _, r := range order {
		if r == pivot {
			res = append(res, target)
		}
		if r == target {
			continue
		}
		res = append(res, r)
	}
	return res
}

func TestRoleOrder_Negative_Conditions(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"unknown id in role_order", func(_ *testing.T, def *Definition) {
			def.RoleOrder = append([]string{"unknown-role"}, def.RoleOrder...)
		}, CodeRef, "role_order[unknown-role]", "role"},
		{"id listed twice", func(_ *testing.T, def *Definition) {
			def.RoleOrder = append(def.RoleOrder, "operator")
		}, CodeRoleOrder, "role_order[operator]", "twice"},
		{"everyone listed", func(_ *testing.T, def *Definition) {
			def.RoleOrder = append(def.RoleOrder, "everyone")
		}, CodeRoleOrder, "role_order[everyone]", "everyone"},
		{"role removed from the list", func(_ *testing.T, def *Definition) {
			def.RoleOrder = without(def.RoleOrder, "guide-coding")
		}, CodeRoleOrder, "roles[guide-coding]", "not listed"},
		{"moderator moved below bot", func(_ *testing.T, def *Definition) {
			def.RoleOrder = moveAfter(def.RoleOrder, "moderator", "bot")
		}, CodeRoleOrder, "role_order[moderator]", "above"},
		{"guide-coding moved below bot", func(_ *testing.T, def *Definition) {
			def.RoleOrder = moveAfter(def.RoleOrder, "guide-coding", "bot")
		}, CodeRoleOrder, "role_order[guide-coding]", "above"},
		{"contributor moved above bot", func(_ *testing.T, def *Definition) {
			def.RoleOrder = moveBefore(def.RoleOrder, "contributor", "bot")
		}, CodeRoleOrder, "role_order[contributor]", "below"},
		{"interest-games moved above bot", func(_ *testing.T, def *Definition) {
			def.RoleOrder = moveBefore(def.RoleOrder, "interest-games", "bot")
		}, CodeRoleOrder, "role_order[interest-games]", "below"},
		{"lang-de moved above bot", func(_ *testing.T, def *Definition) {
			def.RoleOrder = moveBefore(def.RoleOrder, "lang-de", "bot")
		}, CodeRoleOrder, "role_order[lang-de]", "below"},
	})
}

func TestRoleOrder_Boundary_NeighboursOfTheBot(t *testing.T) {
	def := valid(t)
	for _, f := range Validate(def) {
		if f.Code == CodeRoleOrder {
			t.Fatalf("committed order produced unexpected role-order finding: %+v", f)
		}
	}

	t.Run("swap bot with honorary-supporter directly above", func(t *testing.T) {
		d := valid(t)
		swapRoles(d.RoleOrder, "bot", "honorary-supporter")
		var orderFindings []Finding
		for _, f := range Validate(d) {
			if f.Code == CodeRoleOrder {
				orderFindings = append(orderFindings, f)
			}
		}
		if len(orderFindings) != 1 {
			t.Fatalf("expected exactly 1 role-order finding, got %d: %+v", len(orderFindings), orderFindings)
		}
		if orderFindings[0].Path != "role_order[honorary-supporter]" {
			t.Errorf("finding path = %q, want role_order[honorary-supporter]", orderFindings[0].Path)
		}
		if !strings.Contains(orderFindings[0].Message, "above") {
			t.Errorf("finding message %q does not contain 'above'", orderFindings[0].Message)
		}
	})

	t.Run("swap bot with contributor directly below", func(t *testing.T) {
		d := valid(t)
		swapRoles(d.RoleOrder, "bot", "contributor")
		var orderFindings []Finding
		for _, f := range Validate(d) {
			if f.Code == CodeRoleOrder {
				orderFindings = append(orderFindings, f)
			}
		}
		if len(orderFindings) != 1 {
			t.Fatalf("expected exactly 1 role-order finding, got %d: %+v", len(orderFindings), orderFindings)
		}
		if orderFindings[0].Path != "role_order[contributor]" {
			t.Errorf("finding path = %q, want role_order[contributor]", orderFindings[0].Path)
		}
		if !strings.Contains(orderFindings[0].Message, "below") {
			t.Errorf("finding message %q does not contain 'below'", orderFindings[0].Message)
		}
	})
}

func TestRoleOrder_Boundary_EmptyAndBotMissing(t *testing.T) {
	t.Run("role_order nil", func(t *testing.T) {
		d := valid(t)
		d.RoleOrder = nil
		var orderFindings []Finding
		for _, f := range Validate(d) {
			if f.Code == CodeRoleOrder {
				orderFindings = append(orderFindings, f)
			}
		}
		if len(orderFindings) != 26 {
			t.Fatalf("expected 26 role-order findings, got %d: %+v", len(orderFindings), orderFindings)
		}
		for _, f := range orderFindings {
			if !strings.Contains(f.Message, "not listed") {
				t.Errorf("finding %+v message does not contain 'not listed'", f)
			}
		}
	})

	t.Run("bot removed from the list", func(t *testing.T) {
		d := valid(t)
		d.RoleOrder = without(d.RoleOrder, "bot")
		var orderFindings []Finding
		for _, f := range Validate(d) {
			if f.Code == CodeRoleOrder {
				orderFindings = append(orderFindings, f)
			}
		}
		if len(orderFindings) != 1 {
			t.Fatalf("expected exactly 1 role-order finding, got %d: %+v", len(orderFindings), orderFindings)
		}
		if orderFindings[0].Path != "roles[bot]" {
			t.Errorf("path = %q, want roles[bot]", orderFindings[0].Path)
		}
		if !strings.Contains(orderFindings[0].Message, "not listed") {
			t.Errorf("message %q does not contain 'not listed'", orderFindings[0].Message)
		}
		for _, f := range orderFindings {
			if strings.Contains(f.Message, "above") || strings.Contains(f.Message, "below") {
				t.Errorf("unexpected above/below finding when bot is missing: %+v", f)
			}
		}
	})
}

func roleOrderFindings(def *Definition) []Finding {
	var kept []Finding
	for _, f := range Validate(def) {
		if f.Code == CodeRoleOrder {
			kept = append(kept, f)
		}
	}
	return kept
}

func TestRoleOrder_Boundary_SecondListingOfAnID(t *testing.T) {
	t.Run("a second listing below the bot is one finding", func(t *testing.T) {
		def := valid(t)
		def.RoleOrder = append(def.RoleOrder, "operator")
		got := roleOrderFindings(def)
		if len(got) != 1 {
			t.Fatalf("want exactly 1 role-order finding, got %d: %+v", len(got), got)
		}
		if got[0].Path != "role_order[operator]" || !strings.Contains(got[0].Message, "twice") {
			t.Errorf("finding = %+v, want role_order[operator] with 'twice'", got[0])
		}
	})
	t.Run("three listings are one finding", func(t *testing.T) {
		def := valid(t)
		def.RoleOrder = append(def.RoleOrder, "lang-it", "lang-it")
		got := roleOrderFindings(def)
		if len(got) != 1 {
			t.Fatalf("want exactly 1 role-order finding, got %d: %+v", len(got), got)
		}
	})
	t.Run("the first listing decides the position", func(t *testing.T) {
		def := valid(t)
		def.RoleOrder = append([]string{"contributor"}, def.RoleOrder...)
		got := roleOrderFindings(def)
		if !hasFinding(got, CodeRoleOrder, "role_order[contributor]", "below") {
			t.Errorf("no finding for a first listing above the bot: %+v", got)
		}
	})
}

func TestRoleOrder_Boundary_EveryoneHasNoPosition(t *testing.T) {
	for _, by := range []string{"operator", "bot", "choice"} {
		t.Run("everyone assigned by "+by, func(t *testing.T) {
			first := valid(t)
			roleOf(t, first, "everyone").AssignedBy = by
			first.RoleOrder = append([]string{"everyone"}, first.RoleOrder...)
			last := valid(t)
			roleOf(t, last, "everyone").AssignedBy = by
			last.RoleOrder = append(last.RoleOrder, "everyone")
			for _, def := range []*Definition{first, last} {
				for _, f := range roleOrderFindings(def) {
					if strings.Contains(f.Message, "above") || strings.Contains(f.Message, "below") {
						t.Errorf("position finding for the role every member holds: %+v", f)
					}
				}
			}
		})
	}
}

func TestRoleOrder_ChoiceRoleMovedAboveBot(t *testing.T) {
	def := valid(t)
	def.RoleOrder = moveBefore(def.RoleOrder, "interest-games", "bot")
	findings := Validate(def)
	if !hasFinding(findings, CodeRoleOrder, "role_order[interest-games]", "below") ||
		!hasFinding(findings, CodeRoleOrder, "role_order[interest-games]", "choice") {
		t.Errorf("expected finding with 'below' and 'choice', got: %+v", findings)
	}
}
