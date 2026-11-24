# Retri

[English](README.md) | [日本語](README.ja.md)

[![CI](https://github.com/cotta-dev/retri/actions/workflows/ci.yml/badge.svg)](https://github.com/cotta-dev/retri/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/cotta-dev/retri)](https://github.com/cotta-dev/retri/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

SSH ログ収集 & コマンド自動実行ツール

`Retri` という名前は、獲物を持ち帰ることが得意な犬種「レトリーバー」に由来しています。複数のサーバーからログを "取ってくる" 動作そのものを表しています。

Windows での作業でおなじみの **TeraTerm のログ機能＋マクロの組み合わせ** を、WSL（Windows Subsystem for Linux）でも使いたいという動機から生まれました。設定ファイルにコマンドを書いておけば、SSH 接続・コマンド実行・ログ保存を一括で自動化できます。

## 主な機能

* **ローカル作業ログの自動記録**: 引数なしで新しいシェルを起動し、その端末出力をログファイルに記録します（TeraTerm のログ機能相当）。
* **SSH + 作業ログ記録**: ホスト名を引数に指定すると、SSH 接続してセッション全体を自動記録します。
* **コマンド自動実行 & ログ収集**: 設定ファイルに定義したコマンドを複数ホストへ一括実行し、タイムスタンプ付きログとして保存（TeraTerm マクロ相当）。
* **エージェントレス**: 標準の SSH のみ使用。リモートホストへのソフトウェアインストール不要。
* **単一バイナリ**: リリースバイナリは静的リンクです。SSH 接続にはシステムの `ssh` クライアントを使用します。
* **ネットワーク機器対応**: プロンプトと初期化コマンドを設定し、Cisco IOS・Arista EOS・Juniper・Huawei などの対話 CLI に対応できます。
* **文字コードの指定**: デフォルトでは端末描画後の文字バイトを保持し、既知の文字コードから UTF-8 への変換も選択できます。
* **並列実行**: 複数台のサーバーで同時実行できます（デフォルト: 5 台）。
* **SSH Config 対応**: エイリアス、ProxyJump、鍵ファイルなど、`~/.ssh/config` の設定を利用します。

## インストール

リリースパッケージの対象は Linux（`amd64` / `arm64`、WSL を含む）です。
SSH を使うモードには OpenSSH クライアント（`PATH` 上の `ssh`）が必要です。
ローカル／対話 SSH の記録には端末、ローカル記録にはシェルも必要です。

### Ubuntu/Debian（推奨）

Bash で次を実行すると、アーキテクチャに合う最新版の `.deb` を
インストールできます（`curl` が必要です）。

```bash
(
  set -eo pipefail
  cd "$(mktemp -dp /tmp || echo /dev/null)"
  curl -fsS https://api.github.com/repos/cotta-dev/retri/releases/latest |
    grep -Po '"browser_download_url":\s*"\Khttps://github\.com/[^"\s]*_'"$(dpkg --print-architecture)"'\.deb(?=")' |
    xargs -r curl -fsSLO --proto =https
  chmod a+rX . *.deb && sudo apt-get install -y ./*.deb
)
```

手動でダウンロードする場合は、
[Releases ページ](https://github.com/cotta-dev/retri/releases/latest) から、
`dpkg --print-architecture` と一致する `.deb` をダウンロードします。
以下の `VERSION` とアーキテクチャは、実際のファイル名に置き換えてください。

```bash
sudo apt-get install ./retri_VERSION_amd64.deb
```

### ソースからビルド

[go.mod](go.mod) に記載されたバージョン要件を満たす Go を使用します。

```bash
git clone https://github.com/cotta-dev/retri.git
cd retri
CGO_ENABLED=0 go build -o retri -ldflags="-s -w" .
```

### Go でインストール

```bash
CGO_ENABLED=0 go install github.com/cotta-dev/retri@latest
```

## 使い方

### 作業ログを取る（引数なし）

引数なしで実行すると、`$SHELL`（未設定なら `/bin/sh`）を新しいログインシェルとして
起動し、その端末出力を記録します。このシェルを終了すると、Retri を起動した元の
シェルに戻ります。

```bash
retri
# → ~/retri-logs/hostname_YYYYMMDD_HHmmss.log に記録開始
# → exit または Ctrl-D で終了
```

### SSH + 作業ログ（ホスト名を引数に指定）

ホスト名を引数に指定すると、そのホストに SSH 接続し、セッション全体を自動記録します。

```bash
retri myserver
# → myserver に SSH 接続し ~/retri-logs/myserver_YYYYMMDD_HHmmss.log に記録開始
# → exit で切断 & 記録終了
```

このモードの認証と接続には SSH クライアントの設定を使います。
ログには `defaults` とログ関連の CLI オプションを適用しますが、ホスト・グループ・
device type ごとの設定や、Retri による自動認証処理は適用しません。

### コマンドを自動実行してログを収集する

単一ホストでコマンドを実行（`~/.ssh/config` のエイリアスを使用）：

```bash
retri --host myserver --command "df -h"
```

設定ファイルで定義したグループのサーバーでコマンドを実行：

```bash
retri --group web_servers
```

ファイルからコマンドを読み込んで実行（1 行 1 コマンド、空行と `#` で始まる行は無視）：

```bash
retri --host myserver --command-file commands.txt
```

設定ファイルと CLI のコマンドは順番に結合されます。`--command` と `--command-file` は、
設定済みのコマンドを置き換えず追加します。
`--host`、`--group`、`--command`、`--command-file` のいずれかを指定すると自動実行モードに
なり、位置引数のホスト名は無視されます。コマンドがないホストはスキップします。

### コマンドと出力だけを記録する

ローカル／対話 SSH の記録で、commands-only logging を有効にできます。

```bash
retri --log-commands-only
retri --log-commands-only myserver
```

ログインバナー、待機中のプロンプト、入力中の再描画、パスワードプロンプトを除外します。
空 Enter はプロンプトだけの行として残ります。端末画面の表示自体は変わりません。
ネットワーク機器の CLI でも利用できます。

記録セッションのデフォルトとして有効にするには、次のように設定します。

```yaml
defaults:
  log_commands_only: true
```

### コマンドラインオプション

現在のオプションは `retri --help`、一覧は
[オプションリファレンス](docs/cli-options.md) を参照してください。

### シェル補完

`.deb` でインストールすると、bash / zsh / fish の補完ファイルは自動配置されます。
補完ファイルを手動で生成・配置する必要はありません。
シェルの補完機能が有効なら、新しいシェルで自動的に読み込まれます。

ソースビルドや `go install` の場合は、利用しているシェル向けの補完スクリプトを生成して読み込めます：

```bash
# bash
source <(retri --completion bash)
```

zsh では生成した関数を `fpath` 上に保存し、
[`compinit`](https://zsh.sourceforge.io/Doc/Release/Completion-System.html) で登録します。

```zsh
# zsh
mkdir -p ~/.zsh/completions
retri --completion zsh > ~/.zsh/completions/_retri
fpath=(~/.zsh/completions $fpath)
autoload -Uz compinit
compinit
```

以後のシェルでも使うには、`~/.zshrc` の既存の `compinit` 呼び出しより前に
上記の `fpath` の行を設定してください。

```fish
# fish
retri --completion fish | source
```

補完にはオプション説明、オプション値の入力ヒント、`retri <hostname>` 用の
`~/.ssh/config` / `known_hosts` 由来の SSH ホスト候補が含まれます。

## 設定

初回の通常実行時に、`~/.config/retri/config.yaml` と `~/.config/retri/commands/` が
未作成なら自動作成します。別の設定ファイルを読み込むには `--config PATH` を指定します。
`--help`、`--version`、`--config-help`、補完スクリプトの生成では、これらを作成しません。

### `config.yaml` の例

上の使用例で使うグループの最小構成です。`myserver` は SSH のホスト名または
エイリアスに置き換えてください。

```yaml
defaults:
  log_dir: "~/retri-logs"

groups:
  - name: web_servers
    hosts: [myserver]
    commands:
      - "df -h"
      - "uptime"
```

自動実行時の共通設定の優先順位は、低い順に
`defaults` → `groups` → `device_types` → `hosts` → CLI オプションです。
コマンドは各階層の指定を結合します。項目ごとの規則、機器プロファイル、
`switch-[01-04]` のようなホスト範囲指定は、`retri --config-help` または
[設定リファレンス](docs/config-reference.yaml) を参照してください。

### 環境変数とセキュリティ

任意機能の[名前付きcredentialとcache](docs/credentials.ja.md)により、
共有prompt、環境変数、Linux keyring、Bitwardenを利用できます。
通常利用にBitwardenやLinux keyringは必須ではありません。
`user-keyring` / `session-keyring` / `none` の選び方は
[cache backend の比較](docs/credentials.ja.md#cache-backend-の比較)を参照してください。

自動実行時は、平文パスワードの代わりに SSH 鍵や名前付き credential を利用できます。
従来の `password` / `secret` フィールドでは `${VAR}` 展開も使えます
（Retri の実行前に環境変数を設定してください）。

```yaml
defaults:
  password: "${COMMON_SSH_PASSWORD}"
```

フォールバック環境変数（最低優先度）：

| 変数 | 説明 |
| :--- | :--- |
| `RETRI_SSH_PASSWORD` | 自動実行で credential が未設定のときに使う SSH パスワード。 |
| `RETRI_SSH_SECRET` | 自動実行で credential が未設定のときに使う sudo / enable シークレット。 |
| `RETRI_LOG_ENCODING` | `log_encoding` 未指定時に使う出力元文字コード。 |

自動実行では、受信した端末出力を描画する前に、SSH パスワードと sudo / enable
シークレットに文字列が一致した部分をマスクします。コマンドやその出力に
秘密情報を含めないでください。保護範囲は [credential の説明](docs/credentials.ja.md#セキュリティ境界)を参照してください。

## 出力フォーマット

ログはデフォルトで `~/retri-logs` に保存します。`--log-dir` で変更できます。
新規ログディレクトリは `0700`、ログファイルは `0600` の権限で作成します。
既存ログは上書きしません。ログ名はパス区切り文字や制御文字を含まない、
単一のローカルファイル名である必要があります。

端末出力の行には、デフォルトでミリ秒単位のタイムスタンプが付きます。
Retri が挿入するセッションヘッダー・フッター・`[EXEC]`・その区切り線には、
タイムスタンプの接頭辞は付きません。
`--no-timestamp` または `defaults.timestamp: false` で出力行のタイムスタンプを無効にできます。

Linux 自動実行ログの例です。プロンプト・コマンド echo・出力は接続先によって異なります。

ファイル: `example-host_20000101_000000.log`

```text
============================================================
 TARGET HOST : example-host
 DEVICE TYPE : linux
 START TIME  : 2000-01-01 00:00:00
============================================================

----------------------------------------
[EXEC] unset HISTFILE
----------------------------------------
[2000-01-01 00:00:00.100] operator@example-host:~$ unset HISTFILE

----------------------------------------
[EXEC] uptime
----------------------------------------
[2000-01-01 00:00:00.200] operator@example-host:~$ uptime
[2000-01-01 00:00:00.201]  00:00:00 up 1 day,  1 user,  load average: 0.00, 0.00, 0.00

----------------------------------------
[EXEC] whoami
----------------------------------------
[2000-01-01 00:00:00.300] operator@example-host:~$ whoami
[2000-01-01 00:00:00.301] operator
[2000-01-01 00:00:00.802] operator@example-host:~$ exit
[2000-01-01 00:00:00.803] logout

============================================================
 LOG END     : 2000-01-01 00:00:00
============================================================
```

Linux ホストとネットワーク機器の自動実行は、どちらも実際の対話 SSH PTY を
使用します。シェル／CLI のプロンプト、コマンド echo、出力は同じリモート端末
セッションから受信します。ユーザー名・ホスト名・ディレクトリ値から Linux の
疑似プロンプトを生成することはありません。

`device_type: linux`（デフォルト）の自動実行では、最初に `unset HISTFILE` を実行して
記録し、Bash などの永続履歴への保存を抑止します。履歴の挙動は接続先のシェルと
設定に依存します。対話記録モードとネットワーク機器の CLI 履歴は変更しません。

`[EXEC]` は自動実行で送信するコマンドの見出しです。`setup_commands` や
`unset HISTFILE` にも付きますが、最後の終了コマンドには付きません。
検出したプロンプトはバッファーに保持し、見出しの後に端末からのコマンド echo と
ともに記録します。リモート端末が表示していないプロンプトやコマンド echo は生成しません。

ローカル／対話 SSH の記録では、ヘッダーに `TARGET HOST` / `DEVICE TYPE` の代わりに
`SESSION LOG` を使い、`[EXEC]` は挿入しません。

設定した終了コマンドを送信してセッションが閉じた場合、最後のコマンドから
引き継いだリモートシェルの非ゼロ終了値だけでは、セッション全体を SSH 失敗に
しません。SSH 接続障害を示す 255 と signal による終了は引き続き失敗扱いです。

### 端末描画と文字コード

ANSI 制御、CR による再描画、カーソル編集を反映してから行を書き込みます。
タブは展開し、行末空白は削除し、連続する空行はまとめます。
ログは端末描画後のテキストであり、PTY の生ストリームをそのまま保存する形式ではありません。

デフォルトの `log_encoding: raw` は、描画後に残った出力元バイトを文字コード変換せず
保持します。既知の文字コードから UTF-8 へ変換するには、次のように設定します。

```yaml
defaults:
  log_encoding: shift_jis
```

自動実行ではグループ・device type・ホストごとにも設定できます。
ローカル／対話 SSH の記録では `defaults.log_encoding` を使います。
`--log-encoding` は全モードで設定を上書きします。

対応値は `raw`、`utf-8`、`shift_jis`（CP932 / Windows-31J を含む）、
`euc-jp`、`iso-8859-1`、`windows-1252`、`gb18030`、`gbk`、`big5`、`euc-kr` です。
描画後の行が指定した文字コードとして不正な場合は、セッションごとに 1 回警告し、
その行の出力元バイトを同じ `.log` に残します。`.raw` の別ファイルは作成しません。
この場合、そのログには複数の文字コードが混在する可能性があります。

## ライセンス

MIT License で配布しています。詳細は LICENSE を参照してください。
