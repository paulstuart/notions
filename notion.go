//go:generate stringer -type=Command

package notion

import (
	"bufio"
	"container/list"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sync"
	"time"

	"github.com/pkg/errors"
)

var (
	indent = "    "
)

// TODO: address internal IDs vs. external

// Notion represents project, process, or some other collection of intentions
type Notion struct {
	sync.Mutex
	Name, Title string
	Version     Version
	Authors     []Person
	Root        Item
	Created     time.Time
	Modified    time.Time
	lookup      map[int64]*Item
	acls        ACLService
	cmdlog      CmdLogger
	itemID      int64
}

// Print a textual view of a notion to the given writer
func (n *Notion) Print(w io.Writer, number int) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "===========================")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Name:", n.Name)
	if len(n.Title) > 0 {
		fmt.Fprintln(w, "Title:", n.Title)
	}
	fmt.Fprintln(w, "Created:", n.Created)
	fmt.Fprintln(w)
	for i, item := range n.Root.Nodes {
		item.Print(w, i+1, "")
	}
	fmt.Fprintln(w, "===========================")
	fmt.Fprintln(w)
}

func (n *Notion) newItem(parent *Item, text string) *Item {
	item := &Item{parent: parent, Text: text}
	n.Lock()
	n.itemID++
	id := n.itemID
	n.lookup[id] = item
	n.Unlock()
	item.ID = id
	n.cmdlog.CmdSave(DoItemCreate, "id", id, "parent", parent.ID, "text", text)
	return item
}

func (n *Notion) current(item *Item) error {
	if item.ID == 0 {
		return fmt.Errorf("item has no ID")
	}
	current, ok := n.lookup[item.ID]
	if !ok {
		return fmt.Errorf("could not find item with ID: %d", item.ID)
	}
	if current.Version != item.Version {
		return fmt.Errorf("your version %d, current version is %d", item.Version, current.Version)
	}
	return nil
}

func (n *Notion) restricted(item *Item) error {
	if item.ID == 0 {
		return fmt.Errorf("item has no ID")
	}
	current, ok := n.lookup[item.ID]
	if !ok {
		return fmt.Errorf("could not find item with ID: %d", item.ID)
	}
	if current.Version != item.Version {
		return fmt.Errorf("your version %d, current version is %d", item.Version, current.Version)
	}
	return nil
}

func (n *Notion) CmdSave(cmd Command, args ...interface{}) {
	if n.cmdlog != nil {
		n.cmdlog.CmdSave(cmd, args...)
	}
}

// Login grans access to the notion
func (n *Notion) Login(acct Account, person *Person) (*Session, error) {
	if n.acls != nil {
		if err := n.acls.Authenticate(acct); err != nil {
			return nil, err
		}
	}
	return &Session{
		person:  person,
		account: acct,
		notion:  n,
	}, nil
}

// Session represents a user (or API's) interaction with a Notion
type Session struct {
	notion  *Notion
	person  *Person
	account Account
}

type Account interface{}

type ACLService interface {
	Authenticate(Account) error
	Allow(Account, *Item, ItemAction) error
}

type CommandSet struct {
	Command Command
	Args    []interface{}
}

type CmdLogger interface {
	CmdSave(Command, ...interface{}) error
	CmdReader() (chan CommandSet, error)
}

type ioLogger struct {
	w io.Writer
}

func (i *ioLogger) CmdSave(cmd Command, args ...interface{}) error {
	args = append([]interface{}{"CMD:", cmd}, args...)
	fmt.Fprintln(i.w, args...)
	return nil
}

func (i *ioLogger) CmdReader() (chan CommandSet, error) {
	var c chan CommandSet
	return c, nil
}

func TestLogger(w io.Writer) CmdLogger {
	return &ioLogger{w: w}
}

func NewNotion(name string, acls ACLService, logger CmdLogger) (*Notion, error) {
	n := &Notion{
		Name:    name,
		Created: time.Now(),
		Root:    Item{ID: 1},
		acls:    acls,
		cmdlog:  logger,
		lookup:  make(map[int64]*Item),
		itemID:  1,
	}
	n.lookup[n.Root.ID] = &n.Root
	if n.cmdlog != nil {
		n.cmdlog.CmdSave(DoNotionCreate, name)
	}
	return n, nil
}

type Person struct {
	ID        int64
	First     string
	Middle    string
	Last      string
	Suffix    string
	Honorific string
	email     string
	Account   Account
}

type Marker struct {
	ID, Start, End int
}

type Version int64

type Item struct {
	ID                int64   `json:"id"`
	Version           Version `json:"version"`
	Text              string  `json:"text"`
	markers           *list.List
	parent            *Item
	Nodes             []*Item `json:"nodes,omitempty"`
	created, modified time.Time
}

func (i *Item) Print(w io.Writer, number int, prefix string) {
	if len(i.Text) > 0 {
		fmt.Fprint(w, prefix)
		if number > 0 {
			fmt.Fprintf(w, "%d. ", number)
		}
		//fmt.Fprintf(w, "(%d) %s\n", i.ID, i.Text)
		fmt.Fprintf(w, "%s\n", i.Text)
	}
	prefix += indent
	for n, item := range i.Nodes {
		if number > 0 {
			number = n + 1
		}
		item.Print(w, number, prefix)
	}
}

func (i *Item) TextInsert(offset int, text string) error {
	if offset > len(i.Text) {
		return fmt.Errorf("insert is after end of string")
	}

	if offset == 0 {
		i.Text = text + i.Text
	} else if offset == len(i.Text) {
		i.Text += text
	} else {
		i.Text = i.Text[:offset] + text + i.Text[offset:]
		i.reset(offset)
	}
	return nil
}

func (i *Item) TextDelete(offset int, text string) error {
	i.Text = i.Text[:offset] + i.Text[offset+len(text):]
	return nil
}

func (i *Item) first(offset int) *list.Element {
	if i.markers == nil {
		return nil
	}
	for mark := i.markers.Front(); mark != nil; mark = mark.Next() {
		if m := mark.Value.(*Marker); m != nil && m.Start >= offset {
			return mark
		}
	}
	return nil
}

func (i *Item) reset(offset int) {
	for mark := i.first(offset); mark != nil; mark = mark.Next() {
		m := mark.Value.(*Marker)
		if m.Start > offset {
			m.Start += -offset
		}
	}
}

/*
func (i *Item) ItemAppend(item *Item) error {
	i.Nodes = append(i.Nodes, item)
	i.Version++
	fmt.Printf("V:%d T:%s\n", i.Version, i.Text)
	return nil
}

func (i *Item) ItemTruncate() error {
	if len(i.Nodes) == 0 {
		return fmt.Errorf("no nodes to truncate")
	}
	i.Nodes = i.Nodes[:len(i.Nodes)-1]
	return nil
}
*/

func (i *Item) TextAppend(text string) error {
	i.Text += text
	return nil
}

func (i *Item) TextTruncate(text string) error {
	return nil
}

type Command int

const (
	_ Command = iota
	DoNotionCreate
	DoNotionDelete
	DoNotionAppend
	DoNotionTruncate
	DoItemCreate
	DoItemDelete
	DoItemAppend
	DoItemTruncate
	DoItemInsert
	DoItemChild
	DoItemPrune
	DoTextInsert
	DoTextDelete
	DoTextAppend
	DoTextTruncate
	DoStyleTag
	DoStyleDel

	DoSessionItemAppend   // (sessionID, itemID int64, text string)
	DoSessionItemTruncate // (sessionID, itemID int64)
)

type Transactor func(Command, ...interface{}) error

func Transact(cmd Command, args ...interface{}) error {
	return nil
}

var (
	numbered = regexp.MustCompile(`^([0-9]+|[a-z-A-Z])\.\s`)
	indented = regexp.MustCompile(`^\s*`)
)

func (s *Session) NotionJSON(w io.Writer) error {
	if err := s.CanDump(); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("  ", "  ")
	return enc.Encode(s.notion)
}

func (s *Session) Import(r io.Reader) error {
	lastIndent := -1
	var lastItem *Item
	var err error
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if len(s.notion.Title) == 0 {
			s.notion.Title = line
			continue
		}
		indent := indented.FindString(line)
		line = line[len(indent):]
		number := numbered.FindString(line)
		line = line[len(number):]
		if len(line) == 0 {
			continue
		}
		if lastIndent >= 0 && len(indent) > lastIndent {
			lastItem, err = s.ItemAppend(lastItem, line)
		} else {
			lastItem, err = s.ItemAppend(&s.notion.Root, line)
		}
		if err != nil {
			return errors.Wrap(err, "item append error")
		}
		lastIndent = len(indent)
	}

	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "scanner fail")
	}
	return nil
}
