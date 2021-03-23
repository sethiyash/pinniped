// Copyright 2020-2021 the Pinniped contributors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

//nolint: gochecknoinits
func init() {
	rootCmd.AddCommand(generateMarkdownHelpCommand())
}

func generateMarkdownHelpCommand() *cobra.Command {
	return &cobra.Command{
		Args:         cobra.NoArgs,
		Use:          "generate-markdown-help",
		Short:        "Generate markdown help for the current set of non-hidden CLI commands",
		SilenceUsage: true,
		Hidden:       true,
		RunE:         runGenerateMarkdownHelp,
	}
}

func runGenerateMarkdownHelp(cmd *cobra.Command, _ []string) error {
	var generated bytes.Buffer
	if err := generate(&generated); err != nil {
		return err
	}
	if err := write(cmd.OutOrStdout(), &generated, "###### Auto generated by spf13/cobra"); err != nil {
		return err
	}
	return nil
}

func generate(w io.Writer) error {
	if err := generateHeader(w); err != nil {
		return err
	}
	if err := generateCommand(w, rootCmd); err != nil {
		return err
	}
	return nil
}

func generateHeader(w io.Writer) error {
	_, err := fmt.Fprintf(w, `---
title: Command-Line Options Reference
description: Reference for the `+"`pinniped`"+` command-line tool
cascade:
  layout: docs
menu:
  docs:
    name: Command-Line Options
    weight: 30
    parent: reference
---

`)
	return err
}

func generateCommand(w io.Writer, command *cobra.Command) error {
	for _, command := range command.Commands() {
		// if this node is hidden, don't traverse it or its descendents
		if command.Hidden {
			continue
		}

		// generate children
		if err := generateCommand(w, command); err != nil {
			return err
		}

		// generate self, but only if we are a command that people would run to do something interesting
		if command.Run != nil || command.RunE != nil {
			if err := doc.GenMarkdownCustom(command, w, func(_ string) string { return "" }); err != nil {
				return err
			}
		}
	}

	return nil
}

func write(w io.Writer, r io.Reader, unwantedPrefixes ...string) error {
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := s.Text()
		if !containsPrefix(line, unwantedPrefixes) {
			if _, err := fmt.Fprintln(w, line); err != nil {
				return err
			}
		}
	}
	return s.Err()
}

func containsPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
