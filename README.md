# gw

git worktree のラッパーコマンド。ブランチと worktree を絞り込み UI から選んで、作成・移動・削除をする。

絞り込み UI は組み込みなので、fzf などの外部コマンドは要らない。

検索は大文字小文字を区別しない部分文字列一致で、空白で区切った複数の語はすべてを含む候補だけが残る。

## インストール

### mise

```sh
mise use -g github:Alesion30/gw
```

### GitHub Release

[Releases](https://github.com/Alesion30/gw/releases) から実行環境に合った `gw_<os>_<arch>.tar.gz` を取得して、`gw` を PATH の通ったディレクトリに置く。

### go install

```sh
go install github.com/Alesion30/gw/cmd/gw@latest
```

## シェル連携

`gw cd` は子プロセスから親シェルのカレントディレクトリを変えられないため、シェル関数をかぶせる必要がある。

```sh
# ~/.zshrc
eval "$(gw shell-init zsh)"
```

bash と fish も同じ形で使える。

## 使い方

```
gw path [query]              worktree を選んでパスを出力する
gw cd [query]                worktree を選んで移動する（シェル関数が必要）
gw use [query] [--base ref]  ブランチを選んで worktree を作成する
gw remove [query] [--force]  worktree を選んで削除する
gw remove --gone             upstream が消えた worktree をまとめて削除する
gw list [options]            worktree の一覧を表示する
gw copy <file>...            メインの worktree からファイルをコピーする
```

### gw use

worktree 化していないローカルブランチから選んで worktree を作る。作成先は `<repo-root>/.worktrees/<branch>`。

どのブランチにもマッチしない文字列を打ち込んだまま Enter を押すと、その名前で新しいブランチを作るか確認する。起点は `--base` で指定でき、省略するとカレントブランチになる。

```sh
gw use              # 一覧から選ぶ
gw use login        # login で絞り込んだ状態で開く
gw use feat/new --base main
```

### gw remove --gone

`git fetch --prune` を実行したうえで、upstream が消えた（`[gone]`）ブランチの worktree をまとめて削除する。メインの worktree と、いま自分がいる worktree は対象から外す。

```sh
gw remove --gone                  # 確認してから削除
gw remove --gone --delete-branch  # ローカルブランチも一緒に削除
gw remove --gone --force          # 確認なしで削除
```

### gw copy

`.env` のように gitignore していて worktree に持ち越されないファイルを、メインの worktree からコピーする。

```sh
gw copy .env .envrc.local
```

## セットアップスクリプト

リポジトリのルートに `.gw-setup` を置いておくと、`gw use` で worktree を作った直後にその worktree 内で実行する。依存のインストールや `.env` の配置に使う。

```sh
#!/bin/sh
# .gw-setup
gw copy .env
pnpm install
```

`.gw-setup` はリポジトリごとに内容が変わるので、`~/.config/git/ignore` などグローバルな gitignore に入れておくとよい。

## 環境変数

| 変数 | 既定値 | 説明 |
| --- | --- | --- |
| `GW_WORKTREE_DIR` | `<repo-root>/.worktrees` | worktree の作成先 |

## キー操作

| キー | 動作 |
| --- | --- |
| `Enter` | 確定 |
| `Esc` / `Ctrl-C` | 中断 |
| `↑` / `Ctrl-P` | 上へ |
| `↓` / `Ctrl-N` | 下へ |

## 開発

[mise](https://mise.jdx.dev/) でツールを揃える。

```sh
mise install
mise run check      # test + lint
mise run build      # ./gw にビルド
mise run snapshot   # dist/ にリリース成果物を作る
```

## リリース

[Actions の Release ワークフロー](https://github.com/Alesion30/gw/actions/workflows/release.yml)を Run workflow から実行する。ローカルでタグを打つ必要はない。

- `bump` — `patch` / `minor` / `major` から選ぶ。直近のタグを基準に次のバージョンを決める
- `version` — `v1.0.0` のように直接指定したいときだけ埋める。空なら `bump` から算出する

テストが通ったらタグを作成して push し、そのまま GoReleaser がバイナリをビルドして GitHub Release を作る。既存のタグと同じバージョンになる場合は、タグを作る前に失敗する。
