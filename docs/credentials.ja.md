# Credential providers

Retri は既存の `password`、`secret`、`${VAR}`、`RETRI_SSH_PASSWORD`、
`RETRI_SSH_SECRET` をそのまま利用できます。名前付き credential は任意機能であり、
Bitwarden、Vaultwarden、Linux keyring の導入は必須ではありません。

名前付き credential を使うと、**秘密情報をどこから取得するか**と
**どこで利用するか**を分離できます。

```yaml
credentials:
  linux-login:
    provider: prompt
    prompt: "Linux SSH password"

  linux-sudo:
    provider: prompt
    prompt: "Linux sudo password"
    cache:
      backend: session-keyring
      ttl: 8h

defaults:
  password_credential: linux-login
  secret_credential: linux-sudo
```

同じ名前付き credential を複数ホストが使う場合でも、`prompt` は1回の Retri 実行につき
1回だけ表示されます。`session-keyring` cache を有効にすると、同じ Linux ログイン
セッション内の後続の Retri 実行でも TTL が切れるまで再利用できます。

## Provider

### `prompt`

端末から非表示で入力します。

```yaml
credentials:
  common-sudo:
    provider: prompt
    prompt: "Common sudo password"
```

### `env`

環境変数から取得します。既存の環境変数方式と同じ露出特性を持ちます。

```yaml
credentials:
  common-login:
    provider: env
    ref: COMMON_LOGIN_PASSWORD
```

### `keyring`

Linux Session Keyring に既に登録されている `user` key を読み込みます。
Linux kernel APIを直接利用するため、Retriに`keyctl`コマンドは不要です。

```yaml
credentials:
  common-sudo:
    provider: keyring
    ref: retri-linux-sudo
```

### `bitwarden`

公式 Bitwarden CLI の `bw` を使って login item を取得します。Bitwarden Cloud と
Vaultwarden を Retri 側では区別しません。接続先は `bw config server` で決まります。
Retri 自身は vault の login / unlock を行いません。

```yaml
credentials:
  network-login:
    provider: bitwarden
    server: https://vault.bitwarden.com  # must match bw status serverUrl
    account: "BITWARDEN-USER-ID"          # bw status userId (not email)
    ref: 00000000-0000-0000-0000-000000000000
    field: password
```

`field` の既定値は `password` です。`username` と custom field 名も指定できます。

### Vaultwardenの初期設定

Retriは公式の`bw` CLIを利用します。Retriを実行するWSL環境に`bw`をインストールし、
Vaultwardenの外部公開URLを設定してください。

```bash
bw config server https://vault.example.com
bw login your-email@example.com
bw unlock
```

`bw unlock` が表示する Bash 用の`export BW_SESSION=...`を、Retriを実行する同じshellで
実行します。vault itemの内容を表示せず、接続先とunlock状態を確認します。

```bash
bw status
```

出力の`serverUrl`と`userId`を使います。`account`にはメールアドレスではなく`userId`を
指定してください。VaultwardenにLogin itemを作成し、名前で検索してitem IDを取得できます。

```bash
bw list items --search retri
```

Retriの設定でitemを参照します。`field`の既定値は`password`で、`username`またはcustom
field名も指定できます。

```yaml
credentials:
  vaultwarden-ssh:
    provider: bitwarden
    server: https://vault.example.com
    account: "00000000-0000-0000-0000-000000000000"
    ref: "VAULTWARDEN-ITEM-ID"
    field: password

defaults:
  password_credential: vaultwarden-ssh
```

sudoやenable secretが別の場合は、別の名前付きcredentialを作成してください。他のclientで
Vaultwardenのitemを変更した場合は、Retri実行前にCLI vaultを同期します。

```bash
bw sync
```

他のproviderを使うユーザーに`bw`は必要ありません。Retriがlogin、unlock、lock、logout、
syncを自動実行することもありません。

### `literal`

値を Retri の設定ファイルに直接保持します。移行やテスト用途のために用意していますが、
実運用の秘密情報では非推奨です。

```yaml
credentials:
  test-only:
    provider: literal
    value: test-password
```

## Session Keyring cache

Cache は provider とは独立しています。

```yaml
credentials:
  network-login:
    provider: bitwarden
    server: https://vault.bitwarden.com  # must match bw status serverUrl
    account: "BITWARDEN-USER-ID"          # bw status userId (not email)
    ref: 00000000-0000-0000-0000-000000000000
    cache:
      backend: session-keyring
      ttl: 8h
```

Retri は provider にアクセスする前に cache を確認します。そのため一度
Bitwarden/Vaultwarden から取得済みであれば、ネットワーク断や Vaultwarden 停止中でも
key が残っていて TTL 内であれば利用できます。

`session-keyring` は Linux のみ対応し、kernel APIを直接利用します。`ttl` 省略時は8時間です。
ディスク上への永続 cache は作成しません。

明示的に指定した provider が失敗し、有効な cache も存在しない場合、Retri は別の
provider へ勝手に切り替えずエラーを返します。一方、credential 自体が未設定の場合は、
従来どおり非表示の fallback prompt を利用します。

## セキュリティ境界

Session Keyring は、設定ファイル、プロセス引数、継承された環境変数などへの偶発的な
露出を減らします。ただし、Retri が平文パスワードを実際に利用している瞬間まで含めて、
完全な root 権限を持つ攻撃者から秘密を守るものではありません。可能な環境では SSH
password 自体を廃止し、公開鍵認証や FIDO2 を利用する方が強固です。

SSHと記録用shellからは`RETRI_SSH_PASSWORD`、`RETRI_SSH_SECRET`、`BW_*`、
`BWS_ACCESS_TOKEN`に加え、設定内のenv providerが参照する変数と、legacy/literal
credentialの展開に使う変数を除外します。`bw`には`BW_SESSION`など自身の環境を
残します。SSH agent、proxy、localeの通常設定は維持します。親プロセスに存在する
すべてのアプリケーションのsecretを検出する仕組みではありません。

## 解決順序と寿命

優先順位は`defaults < groups < device_types < hosts < CLI`です。複数groupでは
後のgroupが優先されます。`RETRI_SSH_PASSWORD` / `RETRI_SSH_SECRET`は設定未選択時
だけの補完です。legacy `${VAR}`が空に展開された場合も補完できます。同一layerでは
literalと名前付き参照は排他ですが、上位layerはどちらの形式でも上書きできます。
CLI `-p/-s`は展開しないliteralで、プロセス引数として見える可能性があります。

名前付きproviderは空文字・NUL・CR・LFを含む値を返せません。失敗時にlegacy envや
promptへ切り替えません。明示promptにはterminal stdinが必要です。一方、未設定の
credentialは非TTYでは省略し、公開鍵認証やsudo不要の自動実行を妨げません。
`${VAR}`展開の対象はlegacy password/secretとliteralの`value`です。
`ref`、`server`、`account`、`field`、promptラベルは展開しません。

取得値とエラーは名前ごとに1実行内で共有し、端末promptは直列化します。
host実行は取得済みsnapshotを使います。実行中にsession cacheのTTLが切れても、
接続済みSSHや待機中hostのsnapshotを破棄しません。TTLは後続実行の再利用期限です。

session cacheのTTLは既定8h、指定範囲1s〜24hです。読み出しても延長しません。
TTL短縮時は元の取得時刻からの経過時間にも適用します。識別子には設定ファイルの
canonical path、credential名、source、ref、field、Bitwarden profileを含めます。
literal定義の変更は保護されたpayload内で検証し、公開メタデータにはsecretのhashを
出しません。env変数や上流secretの変更だけでは期限内cacheを自動更新しません。

```bash
retri -c config.yaml --credential-cache-refresh -g production
retri -c config.yaml --credential-cache-clear
```

refreshは選択されたcacheを削除して同じsourceから取得し直します。取得失敗時に
旧値を復活させません。clearは現在の定義のcacheを削除し、providerに接続せず終了
します。削除・改名された定義の旧entryは旧TTLで失効します。cache障害はmissとして
隠さずエラーにします。別プロセス間のprompt重複排除は行わず、各snapshotは独自の
期限を持ちます。

Linux session keyringは子プロセスに継承され、PAM/service構成により範囲が異なります。
必ずしも端末や人間のログイン単位ではありません。cache keyは所有者だけに権限を
与えますが、同一ユーザーの別プロセスからの隔離は保証しません。各snapshotの専用
keyringに権限・kernel timeoutを設定してから、内部のvalue keyにsecretを公開します。
value keyにも公開前に権限、公開後にtimeoutを設定します（user key更新はtimeoutを
リセットするため）。途中終了しても外側のringの期限で保持を制限し、失敗時はrevoke
します。平文disk cacheは作りません。Go string/runtime copy、swap、crash dump、
特権によるメモリ参照は防御範囲外です。一時byte bufferは可能な範囲で消去しますが、
完全なメモリ消去は保証しません。

v2 cacheはこの未マージPRの旧実装が書いたentryを利用しません。旧実装を実行済みなら
keyutilsで旧`retri:credential:` keyを確認・削除してください。旧timeout設定失敗に
より期限なしentryが残っている可能性があります。

## Bitwardenの運用

`bw`はBitwarden sourceへの実アクセス時だけ必要です。`--nointeraction`、30秒timeout、
stdout上限を指定し、stderrや応答本文をエラーに出しません。login/unlock/lock/logout/
syncはRetriが管理しません。自身のCLI sessionで`BW_SESSION`をexportし、他clientの
変更が必要なら`bw sync`してください。Retri cacheの失効はCLI vaultの同期を意味
しません。

Bitwardenのsession cacheには`server`と`account`が必要です。`bw status`の`serverUrl`
と`userId`を使ってください（emailではありません）。cache miss時は接続先・account・
unlocked状態を確認してからitemを読みます。hit時は`bw`、unlock、ネットワークが不要
です。vaultのlock/logoutでRetriの独立cacheは消えないため、明示的にclearしてください。
接続先/account変更時は設定も更新すると、別cacheとして扱われます。

自動実行ログは既知credentialを受信chunk境界をまたいで伏せ、terminal描画後にも
伏せます。debugのliteral値も対象です。相手が任意変換したsecretの検出は保証しません。
対話record modeでRetriが取得していないcredentialは伏せられません。
