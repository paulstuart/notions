package notions

import (
	"fmt"
	"io"
	"time"
)

// Session represents a user (or API's) interaction with a Notion
type Session struct {
	ID      int64
	notion  *Notion
	account *Account
	host    string
	created time.Time
}

func (s *Session) Notion() *Notion {
	return s.notion
}

func (s *Session) Print(w io.Writer) {
	s.notion.Print(w, 1)
}

func (s *Session) CanDump() error {
	if s.notion.acls == nil {
		return nil
	}
	return s.notion.acls.Allow(s.account, &s.notion.Root, ItemDump)
}

func (s *Session) GetItem(id int64) (*Item, error) {
	// TODO: separate mutex?
	s.notion.Lock()
	item, ok := s.notion.lookup[id]
	s.notion.Unlock()
	if !ok {
		return nil, fmt.Errorf("no item with ID: %d", id)
	}
	if s.notion.acls != nil {
		if err := s.notion.acls.Allow(s.account, item, ItemRead); err != nil {
			return nil, err
		}
	}
	return item, nil
}

func (s *Session) TextInsert(id int64, offset int, text string) error {
	item, err := s.GetItem(id)
	if err != nil {
		return err
	}
	item.TextInsert(offset, text)
	s.notion.CmdSave(DoTextInsert, "id", id, "offset", offset, "text", text)
	return nil
}

func (s *Session) TextAppend(id int64, text string) error {
	item, err := s.GetItem(id)
	if err != nil {
		return err
	}
	item.TextAppend(text)
	s.notion.CmdSave(DoTextAppend, "id", id, "text", text)
	return nil
}

func (s *Session) TextDelete(id int64, offset int, text string) error {
	item, err := s.GetItem(id)
	if err != nil {
		return err
	}
	item.TextDelete(offset, text)
	s.notion.CmdSave(DoTextDelete, "id", id, "text", text)
	return nil
}

func (s *Session) TextTruncate(id int64, text string) error {
	item, err := s.GetItem(id)
	if err != nil {
		return err
	}
	item.TextTruncate(text)
	s.notion.CmdSave(DoTextTruncate, "id", id, "text", text)
	return nil
}

type ItemAction int

const (
	_ ItemAction = iota
	ItemDump
	ItemRead
	ItemCreate
	ItemModify
	ItemDelete
)

func (s *Session) good(item *Item, action ItemAction) error {
	if item == nil {
		return fmt.Errorf("item is nil")
	}
	// TODO: separate mutex? YESSS!
	s.notion.Lock()
	latest, ok := s.notion.lookup[item.ID]
	s.notion.Unlock()
	if !ok {
		return fmt.Errorf("no item with ID: %d", item.ID)
	}
	if item.Version != latest.Version {
		return fmt.Errorf("old version: %d current: %d", item.Version, latest.Version)
	}
	if s.notion.acls != nil {
		if err := s.notion.acls.Allow(s.account, item, action); err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) ItemAppend(parent *Item, text string) (*Item, error) {
	if err := s.good(parent, ItemCreate); err != nil {
		return nil, err
	}
	item := s.notion.newItem(parent, text)
	parent.Nodes = append(parent.Nodes, item)
	parent.Version++
	return item, nil
}
