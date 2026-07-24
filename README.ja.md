# Retri

[English](README.md) | [日本語](README.ja.md)

[![CI](https://github.com/cotta-dev/retri/actions/workflows/ci.yml/badge.svg)](https://github.com/cotta-dev/retri/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/cotta-dev/retri)](https://github.com/cotta-dev/retri/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

SSH ログ収集 & コマンド自動実行ツール

`Retri` という名前は、獲物を持ち帰ることが得意な犬種「レトリーバー」に由来しています。複数のサーバーからログを "取ってくる" 動作そのものを表しています。

Windows での作業でおなじみの **TeraTerm のログ機能＋マクロの組み合わせ** を、WSL（Windows Subsystem for Linux）でも使いたいという動機から生まれました。設定ファイルにコマンドを書いておけば、SSH 接続・コマンド実行・ログ保存を一括で自動化できます。

## 主な機能

* **ローカル作業ログの自動記録**: 引数なしで実行すると、現在のシェルセッションをそのままログファイルに記録できます（TeraTerm のログ機能相当）。
* **SSH + 作業ログ記録**: ホスト名を引数に指定すると、SSH 接続してセッション全体を自動記録します。
* **コマンド自動実行 & ログ収集**: 設定ファイルに定義したコマンドを複数ホストへ一括実行し、タイムスタンプ付きログとして保存（TeraTerm マクロ相当）。
* **エージェントレス**: 標準の SSH のみ使用。リモートホストへのソフトウェアインストール不要。
* **依存関係なし**: 単一バイナリ（静的リンク）。インストールして即使える。
* **ネットワーク機器対応**: Cisco IOS・Arista EOS・Juniper・Huawei など、PTY 対話が必要な機器も自動処理。パスワードはログに残りません。
* **出力元バイトを保持**: 表示文字の文字コードを推測・変換せず、そのまま保存します。ASCII/ESC端末制御は文字データとは分けて描画処理します。
* **並列実行**: 複数台のサーバーに対して同時実行。台数が増えても待ち時間を最小化。
* **SSH Config 対応**: `~/.ssh/config`（エイリアス、ProxyJump、鍵ファイル等）をそのまま利用可能。

## インストール

### Ubuntu/Debian（推奨）

```bash
curl -fsSL $(curl -fsSL https://api.github.com/repos/cotta-dev/retri/releases/latest \
  | grep browser_download_url | grep "$(dpkg --print-architecture).deb" | cut -d'"' -f4) \
  -o /tmp/retri.deb && sudo apt-get install -y /tmp/retri.deb
```

または [Releases ページ](https://github.com/cotta-dev/retri/releases) から `.deb` を手動でダウンロード：

```bash
cp retri_VERSION_amd64.deb /tmp/
sudo apt-get install -y /tmp/retri_VERSION_amd64.deb
```

### ソースからビルド

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

引数なしで実行すると、現在のシェルセッションの記録を開始します。TeraTerm のログ機能と同様に、打ったコマンドと出力をそのままファイルに残せます。

```bash
retri
# → ~/retri-logs/hostname_YYYYMMDD_HHmmss.log に記録開始
# → exit または Ctrl-D で終了
```

確定して実行したコマンド行とその出力だけを残す場合は、commands-only
logging を有効にします。ログインバナー、待機中のプロンプト、入力中の
再描画、パスワードプロンプトはログから除外されます。空Enterはプロンプト
だけの行として残ります。端末画面の表示自体は変わりません。

```bash
retri --log-commands-only
```

ローカル／SSH作業ログのデフォルトとして設定することもできます：

```yaml
defaults:
  log_commands_only: true
```

### SSH + 作業ログ（ホスト名を引数に指定）

ホスト名を引数に指定すると、そのホストに SSH 接続し、セッション全体を自動記録します。

```bash
retri myserver
# → myserver に SSH 接続し ~/retri-logs/myserver_YYYYMMDD_HHmmss.log に記録開始
# → exit で切断 & 記録終了
```

ネットワーク機器のCLIを含む対話SSHでも利用できます：

```bash
retri --log-commands-only myserver
```

機器の文字コードは推測しません。まずANSI制御、CRによる再描画、カーソル
編集などを端末として反映します。デフォルトの `log_encoding: raw` では、
描画後の各行に残った出力元バイトを、文字コードの解釈・置換・再変換なしで
保存します。証跡ログとして最も忠実な選択ですが、PTYの生ストリーム全体を
1バイトずつ保存するraw captureではありません。端末制御と上書きされた文字
は除去され、タブは展開され、行末空白と連続する空行は正規化されます。機器の
文字コードが既知で可読性を優先する場合は、device type・group・host・
defaults のいずれかで指定すると `.log` をUTF-8へ統一できます：

```yaml
device_types:
  cisco_ios_jp:
    log_encoding: shift_jis
    prompt_regex: "[#>] ?$"
```

対応値は `raw`、`utf-8`、`shift_jis`（CP932 / Windows-31Jを含む）、
`euc-jp`、`iso-8859-1`、`windows-1252`、`gb18030`、`gbk`、`big5`、
`euc-kr` です。`--log-encoding` は設定を上書きします。端末処理後の行が
指定した文字コードとして不正な場合は、警告を1回表示し、その行の変換前
バイトを同じ `.log` に残します。`.raw` の別ファイルは作成しません。
この場合、その `.log` は複数の文字コードが混在する可能性があります。

新規ログは `0600` の非公開権限で作成し、既存ログは上書きしません。
生成されるログ名にパス区切り文字や制御文字が含まれる場合も拒否します。

### コマンドを自動実行してログを収集する

単一ホストでコマンドを実行（`~/.ssh/config` のエイリアスを使用）：
```bash
retri --host myserver --command "df -h"
```

設定ファイルで定義したグループのサーバーでコマンドを実行：
```bash
retri --group web_servers
```

### コマンドラインオプション

[docs/cli-options.md](docs/cli-options.md) を参照してください。

### シェル補完

`.deb` パッケージでインストールした場合、bash / zsh / fish の補完は自動で有効になります。

ソースビルドや `go install` の場合は、利用しているシェル向けの補完スクリプトを生成して読み込めます：

```bash
# bash
source <(retri --completion bash)

# zsh
source <(retri --completion zsh)

# fish
retri --completion fish | source
```

補完にはオプション説明、オプション値の入力ヒント、`retri <hostname>` 用の
`~/.ssh/config` / `known_hosts` 由来の SSH ホスト候補が含まれます。

## 設定

初回実行時に、デフォルトの設定ファイルが `~/.config/retri/config.yaml` に自動作成されます。

### `config.yaml` の例

各セクションの全パラメーターは [docs/config-reference.yaml](docs/config-reference.yaml) を参照してください。

### 環境変数とセキュリティ

設定ファイルにパスワードをハードコードしないでください。`${VAR}` 展開を活用します：

```bash
export COMMON_SSH_PASSWORD="my_secret_password"
```

```yaml
defaults:
  password: "${COMMON_SSH_PASSWORD}"
```

フォールバック環境変数（最低優先度）：

| 変数 | 説明 |
| :--- | :--- |
| `RETRI_SSH_PASSWORD` | 設定ファイルで未指定時の SSH パスワード。 |
| `RETRI_SSH_SECRET` | 設定ファイルで未指定時の Sudo シークレット。 |
| `RETRI_LOG_ENCODING` | `log_encoding` 未指定時に使う出力元文字コード。 |

## 出力フォーマット

ログはデフォルトで `~/retri-logs` に保存されます。

ファイル: `example-host_20000101_000000.log`
```text
============================================================
 TARGET HOST : example-host
 DEVICE TYPE : linux
 START TIME  : 2000-01-01 00:00:00
============================================================
[2000-01-01 00:00:00.100]
[2000-01-01 00:00:00.101] ----------------------------------------
[2000-01-01 00:00:00.101] [EXEC] uptime
[2000-01-01 00:00:00.101] ----------------------------------------
[2000-01-01 00:00:00.102] operator@example-host:~$ uptime
[2000-01-01 00:00:00.103]  00:00:00 up 1 day,  0 user,  load average: 0.00, 0.00, 0.00
[2000-01-01 00:00:00.104]
[2000-01-01 00:00:00.104] ----------------------------------------
[2000-01-01 00:00:00.104] [EXEC] sudo whoami
[2000-01-01 00:00:00.104] ----------------------------------------
[2000-01-01 00:00:00.105] operator@example-host:~$ sudo whoami
[2000-01-01 00:00:00.105] root

============================================================
 LOG END     : 2000-01-01 00:00:00
============================================================
```

Linuxホストとネットワーク機器の自動実行は、どちらも実際の対話SSH PTYを
使用します。シェル／CLIのプロンプト、コマンドecho、出力は同じリモート端末
セッションから受信します。ユーザー名・ホスト名・ディレクトリ値からLinuxの
疑似プロンプトを生成することはありません。

`[EXEC]` 表示の直後に、端末セッションの
`プロンプト + 実行コマンド` を1行で記録します。アイドル状態のプロンプトが
`[EXEC]` より前へ単独で出ることはありません。実際のPTYストリームを端末表示
として描画した結果だけを記録し、別々に受信したプロンプトとコマンドを結合
したり、リモート端末からechoがないコマンド行を生成したりすることはありません。

```text
----------------------------------------
[EXEC] nv con diff
----------------------------------------
[2000-01-01 00:00:00.100] operator@example-switch:mgmt:~$ nv con diff
```

設定した終了コマンドを送信してセッションが閉じた場合、最後のコマンドから
引き継いだリモートシェルの非ゼロ終了値だけでは、セッション全体をSSH失敗に
しません。SSH接続障害を示す255とsignalによる終了は引き続き失敗扱いです。

## ライセンス
MIT License で配布しています。詳細は LICENSE を参照してください。
