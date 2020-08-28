package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/sethvargo/gcs-cacher/cacher"
)

var (
	stdout = os.Stdout
	stderr = os.Stderr

	// bucket is the Cloud Storage bucket.
	bucket string

	// cache is the key to use to cache.
	cache string

	// restore is the list of restore keys to use to restore.
	restore stringSliceFlag

	// allowFailure allows a command to fail.
	allowFailure bool

	// dir is the directory on disk to cache or the destination in which to
	// restore.
	dir string

	// hash is the glob pattern to hash.
	hash string
)

func init() {
	flag.StringVar(&bucket, "bucket", "", "Bucket name without gs:// prefix.")
	flag.StringVar(&dir, "dir", "", "Directory to cache or restore.")

	flag.StringVar(&cache, "cache", "", "Key with which to cache.")
	flag.Var(&restore, "restore", "Keys to search to restore (can use multiple times).")
	flag.BoolVar(&allowFailure, "allow-failure", false, "Allow the command to fail.")
	flag.StringVar(&hash, "hash", "", "Glob pattern to hash.")
}

func main() {
	args := os.Args
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" {
			flag.PrintDefaults()
			os.Exit(0)
		}
	}

	flag.Parse()
	if len(flag.Args()) > 0 {
		fmt.Fprintf(stderr, "no arguments expected\n")
		os.Exit(1)
	}

	switch {
	case cache != "":
		parsed, err := parseTemplate(cache)
		if err != nil {
			fmt.Fprintf(stderr, "%s\n", err)
			os.Exit(1)
		}

		if err := saveCache(bucket, dir, parsed); err != nil {
			fmt.Fprintf(stderr, "%s\n", err)
			if allowFailure {
				os.Exit(0)
			} else {
				os.Exit(1)
			}
		}
	case restore != nil:
		keys := make([]string, len(restore))
		for i, key := range restore {
			parsed, err := parseTemplate(key)
			if err != nil {
				fmt.Fprintf(stderr, "%s\n", err)
				os.Exit(1)
			}
			keys[i] = parsed
		}

		if err := restoreCache(bucket, dir, keys); err != nil {
			fmt.Fprintf(stderr, "%s\n", err)
			if allowFailure {
				os.Exit(0)
			} else {
				os.Exit(1)
			}
		}
	default:
		fmt.Fprintf(stderr, "missing command operation!\n")
		os.Exit(1)
	}
}

func saveCache(bucket, dir, key string) error {
	ctx := context.Background()
	c, err := cacher.New(ctx)
	if err != nil {
		return err
	}

	return c.Save(ctx, &cacher.SaveRequest{
		Bucket: bucket,
		Dir:    dir,
		Key:    key,
	})
}

func restoreCache(bucket, dir string, keys []string) error {
	ctx := context.Background()
	c, err := cacher.New(ctx)
	if err != nil {
		return err
	}

	return c.Restore(ctx, &cacher.RestoreRequest{
		Bucket: bucket,
		Dir:    dir,
		Keys:   keys,
	})
}

func parseTemplate(key string) (string, error) {
	tmpl, err := template.New("").
		Option("missingkey=error").
		Funcs(templateFuncs).
		Parse(key)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var b bytes.Buffer
	if err := tmpl.Execute(&b, nil); err != nil {
		return "", fmt.Errorf("failed to process template: %w", err)
	}
	return b.String(), nil
}

var templateFuncs = template.FuncMap{
	"hashGlob": func(key string) (string, error) {
		return cacher.HashGlob(key)
	},
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}
func (s *stringSliceFlag) Set(value string) error {
	var vals []string
	for _, val := range strings.Split(value, ",") {
		if k := strings.TrimSpace(val); k != "" {
			vals = append(vals, k)
		}
	}
	*s = append(*s, vals...)
	return nil
}
