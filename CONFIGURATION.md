# ShellPort Configuration

ShellPort can be configured through either a JSON configuration file or
environment variables. By default, the configuration loader tries default file
paths first, then falls back to environment variables. The default file paths
are checked in this order:

1. `/etc/shellport/shellport.conf.json`
2. `~/.config/shellport.conf.json`
3. `shellport.conf.json` in the same directory as the running `shellport` binary

If none of those files exists, ShellPort creates
`/etc/shellport/shellport.conf.json` with a minimal writable configuration,
then loads it. The generated file listens on `0.0.0.0:8182` and starts with no
presets so operators can add presets from the UI immediately. `SharedKey` and
`AdminKey` are left empty in the generated file; edit the config file later to
add authentication, admin protection, or other advanced settings.

Use `SHELLPORT_CONFIG` to override the configuration file path:

```sh
SHELLPORT_CONFIG=./shellport.conf.json ./shellport
```

This tells ShellPort to load configuration from `./shellport.conf.json`.

## Configuration File

`shellport.conf.example.json` is an example of a valid configuration file. Use it
as a starting point for your own configuration.

```jsonc
{
  // HTTP Host. Keep it empty to accept request from all hosts, otherwise, only
  // specified host is allowed to access
  "HostName": "localhost",

  // Web interface access password. Set to empty to allow public access to the
  // web interface (bypass the Authenticate page)
  "SharedKey": "WEB_ACCESS_PASSWORD",

  // Optional admin key for admin-only preset config API writes.
  "AdminKey": "",

  // Remote dial timeout. This limits how long of time the backend can spend
  // to connect to a remote host. The max timeout will be determined by
  // server configuration (ReadTimeout).
  // (In Seconds)
  "DialTimeout": 10,

  // Socks5 proxy. When set, ShellPort backend will try to connect remote through
  // the given proxy
  "Socks5": "localhost:1080",

  // Username of the Socks5 server. Please set when needed
  "Socks5User": "",

  // Password of the Socks5 server. Please set when needed
  "Socks5Password": "",

  // Server-side hooks, allowing operators to launch external processes on the
  // server side to influence server behavior
  //
  // The operation of a Hook must be completed within the time limit defined
  // by `HookTimeout` set below. Otherwise it will be terminated, and results
  // a failure for the execution
  //
  // To determine how much time is still left for the execution, a Hook can
  // fetch the deadline information from the `SHELLPORT_HOOK_DEADLINE`
  // environment variable which is a RFC3339 formatted date string indicating
  // after what time the termination will occur
  //
  // Warning: the process will be launched within the same context and system
  // permission which ShellPort is running under, thus is it crucial that the
  // Hook process is designed and operated in a secure manner, otherwise
  // SECURITY VULNERABILITY (commandline injection, for example) maybe created
  // as result
  //
  // Warning: all inputs passed by ShellPort to the hook process must be
  // considered unsanitized, and must be sanitized by each hook themselves
  "Hooks": {
    // before_connecting is called before ShellPort starts to connect to a remote
    // endpoint. If any of the Hook process exited with a non-zero return code,
    // the connection request is aborted
    //
    // This Hook offers two parameters:
    // - SHELLPORT_HOOK_REMOTE_TYPE: Type of the connection (i.e. SSH or Telnet)
    // - SHELLPORT_HOOK_REMOTE_ADDRESS: Address of the remote host
    "before_connecting": [
      // Following example command launches a `/bin/sh` to execute a for loop
      // that prints to Stdout as well as to Stderr
      //
      // Prints to Stdout will be sent to the client side visible to the user,
      // and prints to Stderr will be captured as server side logs and it is
      // invisible to the user (as server logs usually are)
      //
      // The command must be specified in Json array format. Each array element
      // is mapped to a command fragment separated by space. For example:
      // ["command", "-i", "Hello World"] will be mapped to `command -i "Hello
      // World"` before it is executed
      [
        "/bin/sh",
        "-c",
        "for n in $(seq 1 5); do sleep 1 && echo Stdout $SHELLPORT_HOOK_REMOTE_TYPE $n && echo Stderr $SHELLPORT_HOOK_REMOTE_TYPE $n 1>&2; done",
      ],
      // You can add multiple hooks, they're executed in sequence even when the
      // previous one fails
      ["/bin/sh", "-c", "/etc/shellport/before_connecting.sh"],
      ["/bin/another-command", "...", "..."],
    ],
  },

  // The maximum execution time of each hook, in seconds. If this timeout is
  // exceeded, the hook will be terminated, and thus cause a failure
  "HookTimeout": 30,

  // ShellPort HTTP server, you can set multiple ones to serve on different
  // ports
  "Servers": [
    {
      // Which local network interface this server will be listening
      "ListenInterface": "0.0.0.0",

      // Which local network port this server will be listening
      "ListenPort": 8182,

      // Timeout of initial request. HTTP handshake must be finished within
      // this time
      // (In Seconds)
      "InitialTimeout": 10,

      // How long do the connection can stay in idle before the backend server
      // disconnects the client
      // (In Seconds)
      "ReadTimeout": 120,

      // How long the server will wait until the client connection is ready to
      // receive new data. If this timeout is exceeded, the connection will be
      // closed.
      // (In Seconds)
      "WriteTimeout": 120,

      // The interval between internal echo requests
      // (In Seconds)
      "HeartbeatTimeout": 10,

      // Forced delay between each request
      // (In Milliseconds)
      "ReadDelay": 10,

      // Forced delay between each write
      // (In Milliseconds)
      "WriteDelay": 10,

      // Path to TLS certificate file. Set empty to use HTTP
      "TLSCertificateFile": "",

      // Path to TLS certificate key file. Set empty to use HTTP
      "TLSCertificateKeyFile": "",

      // Display a custom title on the Home page
      "ServerTitle": "",

      // Display a short text message on the Home page. Link is supported
      // through `[Title text](https://link.example.com)` format
      "ServerMessage": "",
    },
    {
      "ListenInterface": "0.0.0.0",
      "ListenPort": 8183,
      "InitialTimeout": 3,
    },
  ],

  // Remote Presets, the operator can define presets for users so the user
  // won't have to manually fill-in all the form fields
  //
  // Presets will be displayed in the "Presets" tab on the Connector
  // window
  //
  // Notice: You can use the same JSON value for `SHELLPORT_PRESETS` if you are
  //         configuring your ShellPort through environment variables.
  //
  // Warning: Most Presets Data will be sent to user client WITHOUT any
  //          protection. DO NOT add secret information into Preset except for
  //          Password values that are migrated to Encrypted Password with
  //          SHELLPORT_PRESET_SECRET_KEY.
  "Presets": [
    {
      // Stable preset ID. ShellPort will automatically add missing IDs to
      // file-backed configurations on startup. IDs must be unique.
      "ID": "preset-sdf",

      // Title of the preset
      "Title": "SDF.org Unix Shell",

      // Preset Types, i.e. Telnet, SSH, Mosh, and ET
      "Type": "SSH",

      // Target address and port
      "Host": "sdf.org:22",

      // Define the tab and background color of the console in RGB hex format
      // for better visual identification
      //
      // For example: 110000 will give you a dark red background, 001100 is
      // dark green and 000011 is dark blue
      //
      // The color must not be too bright, as it will make the foreground text
      // hard to read
      "TabColor": "112233",

      // Form fields and values, you have to manually validate the correctness
      // of the field value
      //
      // Defining a Meta field will prevent user from changing it on their
      // Connector Wizard. If you want to allow users to use their own settings,
      // leave the field unset
      //
      // Values in Meta are scheme enabled, and supports following scheme
      // prefixes:
      // - "literal://": Text literal (Default)
      //                 Example: literal://Data value
      //                          (The final value will be "Data value")
      //                 Example: literal://file:///tmp/afile
      //                          (The final value will be "file:///tmp/afile")
      // - "file://": Load Meta value from given file.
      //              Example: file:///home/user/.ssh/private_key
      //                       (The file path is /home/user/.ssh/private_key)
      // - "environment://": Load Meta value from an Environment Variable.
      //                    Example: environment://PRIVATE_KEY_DATA
      //                    (The name of the target environment variable is
      //                    PRIVATE_KEY_DATA)
      //
      // All data in Meta is loaded during start up, and will not be updated
      // even the source already been modified.
      "Meta": {
        // Data for predefined User field
        "User": "pre-defined-username",

        // Data for predefined Encoding field. Valid data is those displayed on
        // the page.
        "Encoding": "pre-defined-encoding",

        // Data for predefined Password field. Use either Password or Encrypted
        // Password, not both. If SHELLPORT_PRESET_SECRET_KEY is set, plaintext
        // Password values are encrypted on startup, written back as Encrypted
        // Password, and removed from the JSON file.
        "Password": "pre-defined-password",

        // Encrypted preset password generated by ShellPort. Do not hand-edit.
        // Requires SHELLPORT_PRESET_SECRET_KEY to decrypt at runtime.
        // "Encrypted Password": "v1:aes-256-gcm:...",

        // Data for predefined Private Key field, should contains the content
        // of a Key file
        "Private Key": "file:///home/user/.ssh/private_key",

        // Data for predefined Authentication field. Valid values is what
        // displayed on the page (Password, Private Key, None)
        "Authentication": "Password",

        // Data for server public key fingerprint. You can acquire the value of
        // the fingerprint by manually connect to a new SSH host with ShellPort,
        // the fingerprint will be displayed on the Fingerprint confirmation
        // page.
        "Fingerprint": "SHA256:bgO....",
      },
    },
    {
      "Title": "Endpoint Telnet",
      "Type": "Telnet",
      "Host": "telnet.example.com:23",
      "Meta": {
        // Data for predefined Encoding field. Valid data is those displayed on
        // the page
        "Encoding": "utf-8",
      },
    },
    {
      "Title": "Example Mosh",
      "Type": "Mosh",
      "Host": "ssh.example.com:22",
      "Meta": {
        "User": "guest",
        "Authentication": "Password",
        // Data for predefined Encoding field. Mosh currently supports utf-8 only.
        "Encoding": "utf-8",
        // Data for predefined Mosh Server field. Defaults to "mosh-server".
        // Provide an executable path only, without command arguments.
        "Mosh Server": "mosh-server",
      },
    },
    {
      "Title": "Example ET",
      "Type": "ET",
      "Host": "ssh.example.com:22",
      "Meta": {
        "User": "guest",
        "Authentication": "Private Key",
        // Data for predefined Encoding field. ET currently supports utf-8 only.
        "Encoding": "utf-8",
        // Data for predefined ET Server Port field. Defaults to "2022".
        "ET Server Port": "2022",
        // Data for predefined ET Command field. Defaults to "et".
        // ShellPort currently only allows the built-in "et" command value.
        "ET Command": "et",
      },
    },
  ],

  // Allow the Preset Remotes only, and refuse to connect to any other remote
  // host
  //
  // NOTICE: You can only configure OnlyAllowPresetRemotes through a config
  //         file. This option is not supported when you are configuring with
  //         environment variables
  "OnlyAllowPresetRemotes": false,
}
```

### Preset Management API

File-backed configurations can update presets without restarting ShellPort:

```http
GET /shellport/config/presets
PUT /shellport/config/presets
```

`GET` returns the current preset list. `PUT` can save a fingerprint for an
existing preset, or replace the full preset list for add/edit/remove clients.
Presets without an `id` are assigned one automatically. Duplicate preset IDs are
rejected.

When authentication is required, `PUT` uses the same time-windowed `X-Key`
authentication format as `/shellport/socket/verify`. The UI supports preset
create, edit, and delete when the active configuration is file-backed and
`OnlyAllowPresetRemotes` is false. If `AdminKey` is configured, the UI prompts
for it on the first protected write and caches it in browser memory until the
page reloads. If `AdminKey` is blank, authenticated users are admin users for
preset management. If both `SharedKey` and `AdminKey` are blank, anonymous
visitors can manage presets.

The preset editor never displays hidden saved passwords. It receives a boolean
that a saved password exists and can keep or clear that password on save.
Fingerprint editing is intentionally not part of the preset editor; users can
save fingerprints from the connection-time fingerprint prompt. Fingerprint saves
require user access and are limited server-side to changing only the selected
preset's `Fingerprint` metadata. When the active configuration was loaded from
environment variables, writes are rejected because there is no JSON file to
update.

Key behavior:

- `SharedKey` and `AdminKey` both set: `SharedKey` is normal UI access,
  `AdminKey` is admin access for protected preset create, edit, and delete.
- `SharedKey` blank and `AdminKey` set: all visitors are users without
  authentication; admin actions require `AdminKey`.
- `SharedKey` set and `AdminKey` blank: anyone who authenticates with
  `SharedKey` has admin access.
- `SharedKey` and `AdminKey` both blank: all visitors have admin access without
  authentication.

### ET Presets

ET presets use the same `Host`, `User`, `Authentication`, `Private Key`,
`Fingerprint`, and `Encoding` metadata as SSH private-key presets. ET v1
requires `Authentication` to be `Private Key`.

ET-specific metadata:

- `ET Server Port`: remote `etserver` TCP port. Defaults to `2022`.
- `ET Command`: local ET client command inside the ShellPort runtime. ShellPort
  currently only allows the built-in `et` value.

ET v1 does not support password authentication or SOCKS5 proxying.

## Environment Variables

Valid environment variables are:

```text
SHELLPORT_HOSTNAME
SHELLPORT_SHAREDKEY
SHELLPORT_ADMIN_KEY
SHELLPORT_DEBUG
SHELLPORT_DIALTIMEOUT
SHELLPORT_SOCKS5
SHELLPORT_SOCKS5_USER
SHELLPORT_SOCKS5_PASSWORD
SHELLPORT_HOOK_BEFORE_CONNECTING
SHELLPORT_HOOKTIMEOUT
SHELLPORT_LISTENPORT
SHELLPORT_INITIALTIMEOUT
SHELLPORT_READTIMEOUT
SHELLPORT_WRITETIMEOUT
SHELLPORT_HEARTBEATTIMEOUT
SHELLPORT_READDELAY
SHELLPORT_WRITEDELAY
SHELLPORT_LISTENINTERFACE
SHELLPORT_TLSCERTIFICATEFILE
SHELLPORT_TLSCERTIFICATEKEYFILE
SHELLPORT_SERVERTITLE
SHELLPORT_SERVERMESSAGE
SHELLPORT_PRESETS
SHELLPORT_ONLYALLOWPRESETREMOTES
SHELLPORT_PRESET_SECRET_KEY
```

These options correspond to their counterparts in the configuration file.

`SHELLPORT_DEBUG` has no JSON counterpart. Set it to any non-empty value to
enable debug-level logs, including sanitized outbound connection attempts,
failures, and disconnect reasons. Docker images write these logs to stdout.

`SHELLPORT_PRESETS` must contain valid JSON-encoded preset data. Its format is
shown in [shellport.conf.example.json](shellport.conf.example.json) and can be loaded with:

```sh
SHELLPORT_PRESETS="$(cat shellport.conf.example.json)" ./shellport
```

`SHELLPORT_PRESET_SECRET_KEY` is optional. When unset, plaintext preset
`Password` values continue to work as before. When set, it must be a
base64-encoded 32-byte key; startup migrates plaintext preset passwords to
`Encrypted Password`, removes the plaintext values from the JSON config file,
and decrypts encrypted preset passwords server-side for SSH/Mosh authentication.
Encrypted preset passwords cannot be used without the same key. The key must be
set through the environment and is rejected if placed in the JSON config file.

When using environment variables, only one ShellPort HTTP server is allowed. Use
the configuration file if you need to serve on multiple ports.

Invalid values in these environment variables are silently reset to defaults
during configuration parsing:

```text
SHELLPORT_DIALTIMEOUT
SHELLPORT_INITIALTIMEOUT
SHELLPORT_READTIMEOUT
SHELLPORT_WRITETIMEOUT
SHELLPORT_HEARTBEATTIMEOUT
SHELLPORT_READDELAY
SHELLPORT_WRITEDELAY
```

Verify these values before starting the instance.
