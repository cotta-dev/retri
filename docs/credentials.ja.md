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
Linux keyutils の `keyctl` コマンドが必要です。

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
    ref: 00000000-0000-0000-0000-000000000000
    field: password
```

`field` の既定値は `password` です。`username` と custom field 名も指定できます。

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
    ref: 00000000-0000-0000-0000-000000000000
    cache:
      backend: session-keyring
      ttl: 8h
```

Retri は provider にアクセスする前に cache を確認します。そのため一度
Bitwarden/Vaultwarden から取得済みであれば、ネットワーク断や Vaultwarden 停止中でも
key が残っていて TTL 内であれば利用できます。

`session-keyring` は Linux のみ対応し、`keyctl` が必要です。`ttl` 省略時は8時間です。
ディスク上への永続 cache は作成しません。

明示的に指定した provider が失敗し、有効な cache も存在しない場合、Retri は別の
provider へ勝手に切り替えずエラーを返します。一方、credential 自体が未設定の場合は、
従来どおり非表示の fallback prompt を利用します。

## セキュリティ境界

Session Keyring は、設定ファイル、プロセス引数、継承された環境変数などへの偶発的な
露出を減らします。ただし、Retri が平文パスワードを実際に利用している瞬間まで含めて、
完全な root 権限を持つ攻撃者から秘密を守るものではありません。可能な環境では SSH
password 自体を廃止し、公開鍵認証や FIDO2 を利用する方が強固です。

Retri は SSH と記録用 shell の子プロセスから `RETRI_SSH_PASSWORD`、
`RETRI_SSH_SECRET`、`BW_SESSION` を除外します。Provider 用プロセスは必要な環境を
別途継承します。
