//go:generate stringer -type=Command

package notions

import (
	"bufio"
	"container/list"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/pkg/errors"
)

var (
	indent = "    "
)

type Schema struct {
	ID        int
	Name, URL string
}

// TODO: address internal IDs vs. external

// Notion represents project, process, or some other collection of intentions
type Notion struct {
	sync.Mutex
	Name     string
	Title    string
	Version  Version
	Authors  []Person
	Root     Item
	Created  time.Time
	Modified time.Time
	lookup   map[int64]*Item
	acls     ACLService
	cmdlog   CmdLogger
	itemID   int64 // track for next item ID
	schemas  []Schema
}

// Print a textual view of a notion to the given writer
func (n *Notion) Print(w io.Writer, number int) {
	fmt.Fprintln(w, "\n===========================\n")
	fmt.Fprintln(w, "Name:", n.Name)
	if len(n.Title) > 0 {
		fmt.Fprintln(w, "Title:", n.Title)
	}
	fmt.Fprintln(w, "Created:", n.Created)
	fmt.Fprintln(w)
	for i, item := range n.Root.Nodes {
		item.Print(w, i+1, "")
	}
	fmt.Fprintln(w, "\n===========================\n")
}

func (n *Notion) newItem(parent *Item, text string) *Item {
	item := &Item{parent: parent, Text: text}
	n.Lock()
	n.itemID++
	id := n.itemID
	n.lookup[id] = item
	n.Unlock()
	item.ID = id
	if n.cmdlog != nil {
		n.cmdlog.CmdSave(DoItemCreate, "id", id, "parent", parent.ID, "text", text)
	}
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

// Login grants access to the notion
func (n *Notion) Login(acct *Account) (*Session, error) {
	if acct == nil {
		return nil, fmt.Errorf("acct is nil")
	}

	if n.acls != nil {
		if err := n.acls.Authenticate(acct); err != nil {
			return nil, err
		}
	}

	return &Session{
		account: acct,
		notion:  n,
	}, nil
}

type People struct {
	sync.RWMutex
	last    int64
	persons map[int64]*Person
	email   map[string]int64
}

func (p People) add(person *Person) {
	if person != nil {
		p.Lock()
		p.last++
		person.ID = p.last
		p.persons[person.ID] = person
		p.email[person.Email] = person.ID
		p.Unlock()
	}
}

//func (p People) FindByEmail(email string) (`   `Y

type Org struct {
	ID       int64
	Name     string
	Parent   *Org
	people   People
	acls     ACLService
	cmdlog   CmdLogger
	sMu      sync.Mutex
	accounts Accounts
}

var (
	orgMu   sync.Mutex
	orgEnum int64
)

func NewOrg(name string, person *Person, parent *Org, acls ACLService, logger CmdLogger) (*Org, error) {
	accts := Accounts{
		acct: make(map[int64]*Account),
	}

	orgMu.Lock()
	orgEnum++
	org := &Org{
		ID:       orgEnum,
		Name:     name,
		Parent:   parent,
		accounts: accts,
	}
	org.people.persons = make(map[int64]*Person)
	org.people.email = make(map[string]int64)

	if parent == nil && person != nil {
		parent = person.Org
	}
	if parent != nil {
		if acls == nil {
			acls = parent.acls
		}
		if logger == nil {
			logger = parent.cmdlog
		}
	}
	org.acls = acls
	org.cmdlog = logger

	// TODO: acls check if person can create org
	return org, nil
}

type Role uint64

const (
	RoleOwner Role = 1 << iota
	RoleAdmin
	RoleEditor
	RoleWriter
	RoleReviewer
	RoleApprover
)

// Account
type Account struct {
	ID      int64
	Person  *Person
	Role    Role
	Created time.Time
}

type Accounts struct {
	sync.RWMutex
	last int64
	acct map[int64]*Account
}

func (a *Accounts) Add(acct *Account) {
	a.Lock()
	a.last++
	acct.ID = a.last
	a.acct[a.last] = acct
	a.Unlock()
}

func (a *Accounts) ByID(id int64) *Account {
	a.RLock()
	acct := a.acct[id]
	a.RUnlock()
	return acct
}

// Generate UUID for authenticating
func (a Account) HashCode() string {
	var uid int64 = -1
	if a.Person != nil {
		uid = a.Person.ID
	}

	text := []byte(fmt.Sprintf("ID:%d UID:$d", a.ID, uid))
	return fmt.Sprintf("%x", sha256.Sum256(text))
}

type Acknowledger interface {
	HashCode() string
}

type ACLService interface {
	Credentials(Acknowledger) (string, string, error)
	Authenticate(Acknowledger) error
	Allow(*Account, *Item, ItemAction) error
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

func (p Person) Notion(name string) (*Notion, error) {
	var acls ACLService
	var cmdlog CmdLogger
	if p.Org != nil {
		acls = p.Org.acls
		cmdlog = p.Org.cmdlog
	}

	n := &Notion{
		Name:    name,
		Created: time.Now(),
		Root:    Item{ID: 1},
		acls:    acls,
		cmdlog:  cmdlog,
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
	Org       *Org
	First     string
	Middle    string
	Last      string
	Suffix    string
	Honorific string
	Email     string
}

type Allowable int

const (
	AllowEmailSearch Allowable = iota + 1
	AllowCreatePerson
)

func (o *Org) Allows(session *Session, action Allowable) bool {
	if o.acls == nil {
		return true
	}
	return true // for NOW
}

func (org *Org) byEmail(email string) (*Person, error) {
	org.people.RLock()
	defer org.people.RUnlock()

	id, ok := org.people.email[email]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	p, ok := org.people.persons[id]
	if ok {
		return p, nil
	}
	// should never happen!
	return nil, fmt.Errorf("email has no matching id")
}

func (org *Org) PersonByEmail(session *Session, email string) (*Person, error) {
	if !org.Allows(session, AllowEmailSearch) {
		return nil, fmt.Errorf("not allowed")
	}

	return org.byEmail(email)
}

/*
type Personable func(*Person)

func FirstName(name string) Personable {
	return func(p *Person) {
		p.First = name
	}
}

func LastName(name string) Personable {
	return func(p *Person) {
		p.Last = name
	}
}

func MiddleName(name string) Personable {
	return func(p *Person) {
		p.Middle = name
	}
}

func Honorific(name string) Personable {
	return func(p *Person) {
		p.Honorific = name
	}
}
*/
//func (o *Org) NewPerson(session *Session, email string, extra ...Personable) (*Person, error) {
func (org *Org) AddPerson(session *Session, person *Person) error {
	if person == nil {
		return fmt.Errorf("person is nil")
	}

	if len(person.Email) == 0 {
		return fmt.Errorf("email is missing")
	}

	if !org.Allows(session, AllowEmailSearch) {
		return fmt.Errorf("email search not allowed")
	}

	if _, err := org.PersonByEmail(session, person.Email); err == nil {
		return fmt.Errorf("user exists with that email")
	}
	if !org.Allows(session, AllowCreatePerson) {
		return fmt.Errorf("not allowed")
	}

	org.people.add(person)
	return nil
}

func (org *Org) NewAccount(person *Person, role Role) (*Account, error) {
	acct := &Account{
		Person: person,
		Role:   role,
	}
	org.accounts.Add(acct)
	return acct, nil
}

type Marker struct {
	ID, Start, End int
}

type Version int64

type Comment struct {
	AuthorID int64     // Person.ID
	Text     string    // TODO: make rich text struct
	Re       int       // offset into text the comment is citing
	Len      int       // length text the comment is citing
	Comments []Comment // so meta
	Created  time.Time
	Modified time.Time
}

type Item struct {
	ID                int64   `json:"id"`
	Version           Version `json:"version"`
	Text              string  `json:"text"`
	markers           *list.List
	parent            *Item
	Nodes             []*Item `json:"nodes,omitempty"`
	Comments          []Comment
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
	if len(text) > len(i.Text) {
		return fmt.Errorf("too much deletin'")
	}
	offset := len(i.Text) - len(text)
	fmt.Printf("%s vs %s (%t)\n", text, i.Text[offset:], text == i.Text[offset:])
	if text != i.Text[offset:] {
		return fmt.Errorf("%s does not match %s", text, i.Text[offset:])
	}
	i.Text = i.Text[:offset]
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

func (n *Notion) Serve(port int) {
	err := loadTemplates("assets/templates")
	if err != nil {
		panic(err)
	}
	http.HandleFunc("/text", func(w http.ResponseWriter, r *http.Request) {
		n.Print(w, 1)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		renderTemplate(w, r, "index", n)
	})

	conn := fmt.Sprintf(":%d", port)
	log.Fatal(http.ListenAndServe(conn, nil))
}

/*
func (n *Notion) html(file string) error {
	tmpl, err := template.ParseGlob("assets/*.html")
	if err != nil {
		return err
	}

}
*/

//func (n *Notion) RenderHtml(w http.ResponseWriter)

type Dummy struct{}

func (d Dummy) Credentials(Acknowledger) (string, string, error) {
	return "username", "password", nil
}

func (d Dummy) Authenticate(a Acknowledger) error {
	return nil
}

func (d Dummy) Allow(acct *Account, item *Item, action ItemAction) error {
	return nil
}
