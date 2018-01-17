package component

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
)

func splitSlug(s string) []string {
	s = strings.Replace(s, "_", "|", -1)
	s = strings.Replace(s, "|||", "|_|", -1)
	return strings.Split(s, "|")
}

func NewResourceParser() *ResourceParser {
	r := ResourceParser{}
	r.categories.index = make(map[[2]string]int)
	r.categories.list = make([]*Category, 0)
	r.forms = make(map[[2]string]*Form)
	return &r
}

type ResourceParser struct {
	buffer     bytes.Buffer
	categories struct {
		index map[[2]string]int
		list  []*Category
	}
	forms map[[2]string]*Form
}

func (r *ResourceParser) addCat(c *Category) {
	r.categories.index[[2]string{c.ID, c.Locale}] = len(r.categories.list)
	r.categories.list = append(r.categories.list, c)
}

func (r *ResourceParser) getCat(id, locale string) *Category {
	if len(r.categories.index) == 0 {
		return nil
	}
	idx := r.categories.index[[2]string{id, locale}]
	return r.categories.list[idx]
}

func (r *ResourceParser) Categories() map[string][]*Category {
	var res = make(map[string][]*Category)
	for _, cat := range r.categories.list {
		res[cat.Locale] = append(res[cat.Locale], cat)
	}
	return res
}

func (r *ResourceParser) Parse(cmp Component, res *Resource, locale string) error {
	switch v := cmp.(type) {
	case *Form:
		return r.parseForm(v, res, locale)
	case *Category:
		return r.parseCategory(v, res, locale)
	case *Subcategory:
		return r.parseSubcategory(v, res, locale)
	case *Difficulty:
		return r.parseDifficulty(v, res, locale)
	case *Item:
		return r.parseItem(v, res, locale)
	case *Checklist:
		return r.parseChecklist(v, res, locale)
	default:
		return errors.New("Invalid Component")
	}
}

func (r *ResourceParser) parseForm(f *Form, res *Resource, locale string) error {
	var newForm = Form{
		ID:      f.ID,
		Name:    res.Content[0]["form"],
		Locale:  locale,
		Screens: make([]FormScreen, len(f.Screens)),
	}
	m := res.Content[1:]
	for i := range newForm.Screens {
		screen := &newForm.Screens[i]
		screen.Items = make([]FormInput, len(f.Screens[i].Items))
		if f.Screens[i].Name != "" {
			if len(m) == 0 {
				return fmt.Errorf("No more at screen %d/%d", i+1, len(f.Screens))
			}
			if name := m[0]["screen"]; name != "" {
				screen.Name = name
				m = m[1:]
			} else {
				return fmt.Errorf("Expected screen %d, got item", i)
			}
		}
		for j := range screen.Items {
			item := screen.Items[j]
			if item.Label == "" && item.Hint == "" && item.Options == nil {
				continue
			}
			if s := m[0]["screen"]; s != "" {
				return fmt.Errorf("Expected item %d/%d, got screen %q", i, j, s)
			}
			item.Label, item.Hint = m[0]["label"], m[0]["hint"]
			item.Options = strings.Split(m[0]["options"], ";")
			m = m[1:]
		}
	}
	r.forms[[2]string{newForm.ID, newForm.Locale}] = &newForm
	return nil
}

func (r *ResourceParser) parseCategory(c *Category, res *Resource, locale string) error {
	if len(res.Content) != 1 {
		return ErrContent
	}
	r.addCat(&Category{
		ID:     c.ID,
		Order:  c.Order,
		Name:   res.Content[0]["name"],
		Locale: locale,
	})
	return nil
}

func (r *ResourceParser) parseSubcategory(s *Subcategory, res *Resource, locale string) error {
	if len(res.Content) != 1 {
		return ErrContent
	}
	cat := r.getCat(s.parent.ID, locale)
	if cat == nil {
		return fmt.Errorf("No cat %q (%s)", s.parent.ID, locale)
	}
	cat.Add(&Subcategory{
		ID:    s.ID,
		Order: s.Order,
		Name:  res.Content[0]["name"],
	})
	return nil
}

func (r *ResourceParser) parseDifficulty(d *Difficulty, res *Resource, locale string) error {
	if len(res.Content) != 1 {
		return ErrContent
	}
	cat := r.getCat(d.parent.parent.ID, locale)
	if cat == nil {
		return fmt.Errorf("No cat %q (%s)", d.parent.parent.ID, locale)
	}
	sub := cat.Sub(d.parent.ID)
	if sub == nil {
		return fmt.Errorf("No sub %q (%s)", d.parent.ID, locale)
	}
	sub.AddDifficulty(&Difficulty{
		ID:    d.ID,
		Descr: res.Content[0]["description"],
	})
	return nil
}

func (r *ResourceParser) parseItem(i *Item, res *Resource, locale string) error {
	if len(res.Content) == 0 {
		return ErrContent
	}
	cat := r.getCat(i.parent.parent.parent.ID, locale)
	if cat == nil {
		return fmt.Errorf("No cat %q (%s)", i.parent.parent.ID, locale)
	}
	sub := cat.Sub(i.parent.parent.ID)
	if sub == nil {
		return fmt.Errorf("No sub %q (%s)", i.parent.parent.ID, locale)
	}
	dif := sub.Difficulty(i.parent.ID)
	if dif == nil {
		return fmt.Errorf("No dif %q (%s)", i.parent.ID, locale)
	}
	item := &Item{
		ID:    i.ID,
		Title: strings.TrimSpace(res.Content[0]["title"]),
		Order: i.Order,
	}
	r.buffer.Reset()
	// Old Verion Compatibility
	if res.Content[0]["body"] != "" {
		if len(res.Content) != 1 {
			return fmt.Errorf("Invalid Legacy %q (%s)", i.parent.ID, locale)
		}
		r.buffer.WriteString(strings.TrimSpace(res.Content[0]["body"]))
	} else {
		for _, v := range res.Content[1:] {
			if r.buffer.Len() != 0 {
				r.buffer.WriteString(paragraphSep)
			}
			r.buffer.WriteString(strings.TrimSpace(v["body"]))
		}
	}
	item.Body = r.buffer.String()
	dif.AddItem(item)
	return nil
}

func (r *ResourceParser) parseChecklist(c *Checklist, res *Resource, locale string) error {
	cat := r.getCat(c.parent.parent.parent.ID, locale)
	if cat == nil {
		return fmt.Errorf("No cat %q (%s)", c.parent.parent.ID, locale)
	}
	sub := cat.Sub(c.parent.parent.ID)
	if sub == nil {
		return fmt.Errorf("No sub %q (%s)", c.parent.parent.ID, locale)
	}
	dif := sub.Difficulty(c.parent.ID)
	if dif == nil {
		return fmt.Errorf("No dif %q (%s)", c.parent.ID, locale)
	}

	for len(res.Content) > 0 && res.Content[0] == nil {
		res.Content = res.Content[1:]
	}
	if l, e := len(res.Content), len(c.Checks); l != e {
		return fmt.Errorf("%d checks, %s expected", l, e)
	}

	var checks Checklist
	for i, r := range res.Content {
		checks.Add(Check{
			Text:    strings.TrimSpace(r["text"]),
			NoCheck: c.Checks[i].NoCheck,
		})
	}
	dif.SetChecks(&checks)
	return nil
}
