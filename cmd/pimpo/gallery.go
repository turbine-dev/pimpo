package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/turbine-dev/pimpo/internal/gallery"
	"github.com/turbine-dev/pimpo/internal/routine"
)

const galleryUsage = `usage:
  pimpo gallery keygen --out KEYFILE
  pimpo gallery build --key KEYFILE --author ID --name "Your Name" DIR   (signs DIR/routines/*.json into DIR/index.json)
  pimpo gallery sign-authors --key ROOTKEYFILE DIR                       (maintainers: signs the authors in DIR/index.json with the gallery root key)
  pimpo gallery verify INDEX                                             (file or URL; fails on any problem)`

func galleryCmd(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(galleryUsage)
	}
	fs := flag.NewFlagSet("gallery", flag.ContinueOnError)
	keyFile := fs.String("key", "", "author private key file")
	outFile := fs.String("out", "", "where to write the new private key")
	author := fs.String("author", "", "author id in the index")
	name := fs.String("name", "", "author display name")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	switch args[0] {
	case "keygen":
		if *outFile == "" {
			return errors.New(galleryUsage)
		}
		pub, priv, err := gallery.Keygen()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(*outFile), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(*outFile, []byte(priv+"\n"), 0o600); err != nil {
			return err
		}
		fmt.Fprintf(out, "Private key written to %s (keep it; it signs your routines).\nPublic key: %s\n", *outFile, pub)
		return nil
	case "build":
		if *keyFile == "" || *author == "" || fs.NArg() != 1 {
			return errors.New(galleryUsage)
		}
		return galleryBuild(fs.Arg(0), *keyFile, *author, *name, out)
	case "sign-authors":
		if *keyFile == "" || fs.NArg() != 1 {
			return errors.New(galleryUsage)
		}
		return gallerySignAuthors(fs.Arg(0), *keyFile, out)
	case "verify":
		if fs.NArg() != 1 {
			return errors.New(galleryUsage)
		}
		return galleryVerify(fs.Arg(0), out)
	}
	return errors.New(galleryUsage)
}

func galleryBuild(dir, keyFile, author, name string, out io.Writer) error {
	raw, err := os.ReadFile(keyFile)
	if err != nil {
		return err
	}
	priv := strings.TrimSpace(string(raw))
	pub, err := gallery.PublicKey(priv)
	if err != nil {
		return err
	}
	indexPath := filepath.Join(dir, "index.json")
	ix := gallery.Index{Authors: map[string]gallery.Author{}}
	if b, err := os.ReadFile(indexPath); err == nil {
		if err := json.Unmarshal(b, &ix); err != nil {
			return fmt.Errorf("%s: %w", indexPath, err)
		}
	}
	if ix.Authors == nil {
		ix.Authors = map[string]gallery.Author{}
	}
	if a, ok := ix.Authors[author]; ok && a.Key != pub {
		return fmt.Errorf("author %q is listed with another key", author)
	}
	if name == "" {
		name = ix.Authors[author].Name
	}
	ix.Authors[author] = gallery.Author{Name: name, Key: pub, URL: ix.Authors[author].URL}
	files, _ := filepath.Glob(filepath.Join(dir, "routines", "*.json"))
	sort.Strings(files)
	byID := map[string]gallery.Entry{}
	for _, e := range ix.Entries {
		byID[e.ID] = e
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		var r routine.Routine
		if err := json.Unmarshal(b, &r); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		id := strings.TrimSuffix(filepath.Base(f), ".json")
		if old, ok := byID[id]; ok && old.Author != author {
			return fmt.Errorf("%s belongs to %s", id, old.Author)
		}
		if old, ok := byID[id]; ok && old.Hash == gallery.Hash(r) {
			continue
		}
		e, err := gallery.Sign(id, author, r, priv)
		if err != nil {
			return err
		}
		byID[id] = e
		fmt.Fprintf(out, "signed %s %s\n", id, e.Hash[:12])
	}
	ix.Entries = ix.Entries[:0]
	for _, e := range byID {
		ix.Entries = append(ix.Entries, e)
	}
	sort.Slice(ix.Entries, func(i, j int) bool { return ix.Entries[i].ID < ix.Entries[j].ID })
	if err := writeIndex(indexPath, ix); err != nil {
		return err
	}
	if err := ix.CheckAuthors(gallery.RootKeys); err != nil {
		// A new author, or a changed one, waits for a maintainer.
		fmt.Fprintf(out, "%v: a gallery maintainer signs the authors (pimpo gallery sign-authors) after reviewing them.\n", err)
		return verifyEntries(ix, out)
	}
	return galleryVerify(indexPath, out)
}

// gallerySignAuthors signs the authors of DIR/index.json with a gallery
// root key, after printing who they are.
func gallerySignAuthors(dir, keyFile string, out io.Writer) error {
	raw, err := os.ReadFile(keyFile)
	if err != nil {
		return err
	}
	pub, err := gallery.PublicKey(strings.TrimSpace(string(raw)))
	if err != nil {
		return err
	}
	if !slices.Contains(gallery.RootKeys, pub) {
		return fmt.Errorf("%s is not a gallery root key this Pimpo knows", keyFile)
	}
	indexPath := filepath.Join(dir, "index.json")
	b, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}
	var ix gallery.Index
	if err := json.Unmarshal(b, &ix); err != nil {
		return fmt.Errorf("%s: %w", indexPath, err)
	}
	ids := make([]string, 0, len(ix.Authors))
	for id := range ix.Authors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		fmt.Fprintf(out, "author %-16s %s %s\n", id, ix.Authors[id].Key, ix.Authors[id].Name)
	}
	if ix, err = gallery.SignAuthors(ix, strings.TrimSpace(string(raw))); err != nil {
		return err
	}
	if err := writeIndex(indexPath, ix); err != nil {
		return err
	}
	fmt.Fprintf(out, "signed %d authors\n", len(ids))
	return galleryVerify(indexPath, out)
}

func writeIndex(path string, ix gallery.Index) error {
	b, _ := json.MarshalIndent(ix, "", " ")
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func galleryVerify(src string, out io.Writer) error {
	ix, err := gallery.Load(context.Background(), src)
	if err != nil {
		return err
	}
	return verifyEntries(ix, out)
}

func verifyEntries(ix gallery.Index, out io.Writer) error {
	ctx := context.Background()
	bad := 0
	for _, e := range ix.Entries {
		rep := ix.Verify(ctx, e)
		if rep.Verified {
			fmt.Fprintf(out, "ok    %-28s uses %s\n", e.ID, strings.Join(rep.Uses, ", "))
			continue
		}
		bad++
		fmt.Fprintf(out, "FAIL  %-28s %s\n", e.ID, strings.Join(rep.Problems, "; "))
	}
	fmt.Fprintf(out, "%d routines, %d with problems\n", len(ix.Entries), bad)
	if bad > 0 {
		return fmt.Errorf("%d routines failed verification", bad)
	}
	return nil
}
