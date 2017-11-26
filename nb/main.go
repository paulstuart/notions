package main

import (
	"fmt"
	"os"

	"github.com/paulstuart/notions"
)

const (
	testfile = "../todolist.txt"
)

var (
	acct   *notions.Account
	ourOrg *notions.Org
	mySelf = &notions.Person{
		First: "Paul",
		Last:  "Stuart",
		Email: "pauleyphonic@gmail.com",
	}
)

func init() {
	var err error
	dummy := &notions.Dummy{}
	logger := notions.TestLogger(os.Stderr)
	ourOrg, err = notions.NewOrg("The Corporation aka a TLN", mySelf, nil, dummy, logger)
	if err != nil {
		panic(err)
	}
	if err := ourOrg.AddPerson(nil, mySelf); err != nil {
		panic(err)
	}

	acct, err = ourOrg.NewAccount(mySelf, notions.RoleOwner)
	if err != nil {
		panic(err)
	}
}

func readme() {
	f, err := os.Open(testfile)
	if err != nil {
		panic(err)
	}
	s := testSession()
	if err := s.Import(f); err != nil {
		panic(err)
	}
	if err := s.TextAppend(4, " real soon"); err != nil {
		panic(err)
	}
	if err := s.TextDelete(7, 7, " ant"); err != nil {
		panic(err)
	}
	if err := s.TextTruncate(2, " up"); err != nil {
		panic(err)
	}
	if err := s.TextInsert(5, 1, "ave s"); err != nil {
		panic(err)
	}
	if err := s.TextTruncate(5, "x for later"); err != nil {
		panic(err)
	}
	//s.NotionJSON(os.Stdout)
	fmt.Println("STDOUT:")
	s.Print(os.Stdout)
	fmt.Println("EXIT!")
	os.Exit(0)
}

func showme(name string) {
	f, err := os.Open(name)
	if err != nil {
		panic(err)
	}
	s := testSession()
	if err := s.Import(f); err != nil {
		panic(err)
	}
	if err := s.TextAppend(4, " real soon"); err != nil {
		panic(err)
	}
	if err := s.TextDelete(7, 7, " ant"); err != nil {
		panic(err)
	}
	if err := s.TextTruncate(2, " up"); err != nil {
		panic(err)
	}
	if err := s.TextInsert(5, 1, "ave s"); err != nil {
		panic(err)
	}
	if err := s.TextTruncate(5, "x for later"); err != nil {
		panic(err)
	}
	//s.NotionJSON(os.Stdout)
	s.Notion().Serve(8080)
	os.Exit(0)
}

func testSession() *notions.Session {
	n, err := mySelf.Notion("whatever")
	if err != nil {
		panic(err)
	}
	s, err := n.Login(acct)
	if err != nil {
		panic(err)
	}
	return s
}

func main() {
	showme(testfile)
	readme()

	//fmt.Println("here we go!")
	//l := notions.TestLogger(os.Stdout)
	n, err := mySelf.Notion("ok, whatever")
	if err != nil {
		panic(err)
	}

	s, err := n.Login(acct)
	if err != nil {
		panic(err)
	}
	first, err := s.ItemAppend(&n.Root, "line one")
	if err != nil {
		panic(err)
	}
	if _, err := s.ItemAppend(&n.Root, "number 2"); err != nil {
		panic(err)
	}
	if _, err := s.ItemAppend(&n.Root, "third time"); err != nil {
		panic(err)
	}
	if _, err := s.ItemAppend(first, "first child"); err != nil {
		panic(err)
	}

	n.Print(os.Stdout, 1)
	s.NotionJSON(os.Stdout)
	fmt.Println("DONE!")
}
