# Retri

[English](README.md) | [日本語](README.ja.md)

[![CI](https://github.com/cotta-dev/retri/actions/workflows/ci.yml/badge.svg)](https://github.com/cotta-dev/retri/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/cotta-dev/retri)](https://github.com/cotta-dev/retri/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

SSH ログ収集 & コマンド自動実行ツール

`Retri` という名前は、獲物を持ち帰ることが得意な犬種「レトリーバー」に由来しています。複数のサーバーからログを "取ってくる" 動作そのものを表しています。

Windows での作業でおなじみの **TeraTerm のログ機能＋マクロの組み合わせ** を、WSL（Windows Subsystem for Linux）でも使いたいという動機から生まれました。設定ファイルにコマンドを書いておけば、SSH 接続・コマンド実行・ログ保存を一括で自動化できます。

## 主な機能

* **作業ログの自動記録**: オプションなしで実行すると、現在のシェルセッションをそのままログファイルに記録できます（TeraTerm のログ機能相当）。
* **コマンド自動実行 & ログ収集**: 設定ファイルに定義したコマンドを複数ホストへ一括実行し、タイムスタンプ付きログとして保存（TeraTerm マクロ相当）。
* **エージェントレス**: 標準の SSH のみ使用。リモートホストへのソフトウェアインストール不要。
* **依存関係なし**: 単一バイナリ（静的リンク）。インストールして即使える。
* **ネットワーク機器対応**: Cisco IOS・Arista EOS・Juniper・Huawei など、PTY 対話が必要な機器も自動処理。パスワードはログに残りません。
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

### 作業ログを取る（オプションなし）

引数なしで実行すると、現在のシェルセッションの記録を開始します。TeraTerm のログ機能と同様に、打ったコマンドと出力をそのままファイルに残せます。

```bash
retri
# → ~/retri-logs/hostname_YYYYMMDD_HHmmss.log に記録開始
# → exit または Ctrl-D で終了
```

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

## 出力フォーマット

ログはデフォルトで `~/retri-logs` に保存されます。

ファイル: `myserver_20251129_120000.log`
```text
============================================================
 TARGET HOST : myserver
 DEVICE TYPE : linux
 START TIME  : 2025-11-29 12:00:00
============================================================

[2025-11-29 12:00:01.123] --- EXEC: df -h ---
[2025-11-29 12:00:01.150] Filesystem      Size  Used Avail Use% Mounted on
[2025-11-29 12:00:01.150] /dev/sda1        50G   10G   40G  20% /

[2025-11-29 12:00:01.155] --- EXEC: uptime ---
[2025-11-29 12:00:01.160]  12:00:01 up 10 days,  4:20,  1 user,  load average: 0.05, 0.03, 0.01

============================================================
 LOG END     : 2025-11-29 12:00:02
============================================================
```

## ライセンス
MIT License で配布しています。詳細は LICENSE を参照してください。
