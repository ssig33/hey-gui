package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// lang is "ja" or "en". HEY_GUI_LANG overrides the macOS language setting.
var lang = detectLang()

func detectLang() string {
	if v := os.Getenv("HEY_GUI_LANG"); v != "" {
		return v
	}
	out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.Trim(strings.TrimSpace(line), `",`)
			if line == "" || line == "(" || line == ")" {
				continue
			}
			if strings.HasPrefix(line, "ja") {
				return "ja"
			}
			return "en"
		}
	}
	if strings.HasPrefix(os.Getenv("LANG"), "ja") {
		return "ja"
	}
	return "en"
}

// T translates an English format string and formats it with args.
func T(s string, args ...any) string {
	if lang == "ja" {
		if t, ok := ja[s]; ok {
			s = t
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

var ja = map[string]string{
	"(No subject)":                   "(件名なし)",
	"%d / %d threads":                "%d / %d 件",
	"%d threads":                     "%d 件",
	"Checking":                       "確認中",
	"Checking hey CLI…":              "hey CLI を確認しています…",
	"Couldn't mark as seen: %s":      "既読にできませんでした: %s",
	"Error":                          "エラー",
	"Example: /opt/homebrew/bin/hey": "例: /opt/homebrew/bin/hey",
	"hey command not found. Set its location below.": "hey コマンドが見つかりません。下で場所を指定してください。",
	"hey exited with code %d: %s":                    "hey が終了コード %d で失敗しました: %s",
	"hey location (empty to search automatically)":   "hey の場所 (空なら自動で探す)",
	"hey not found":     "hey なし",
	"Loading Imbox…":    "Imbox を読み込んでいます…",
	"Loading the rest…": "残りを読み込み中",
	"Logged in":         "ログイン中",
	"Logged out":        "未ログイン",
	"Newest first":      "新しい順",
	"Not logged in. Run `hey auth login` in Terminal.": "ログインしていません。ターミナルで `hey auth login` を実行してください。",
	"Oldest first":             "古い順",
	"Open in Browser":          "ブラウザで開く",
	"Open This Email":          "このメールを開く",
	"Open HTML":                "HTML を開く",
	"Press r to check again.":  "r で再確認します。",
	"Reload with This Setting": "この設定で読み直す",
	"Search (/)":               "検索 (/)",
	"Select a thread":          "スレッドを選んでください",
	"Server search: %d":        "サーバ検索 %d 件",
	"Unread %d":                "未読 %d",
	"Unread only (u)":          "未読のみ (u)",
	"Mon":                      "月",
	"Tue":                      "火",
	"Wed":                      "水",
	"Thu":                      "木",
	"Fri":                      "金",
	"Sat":                      "土",
	"Sun":                      "日",
}
