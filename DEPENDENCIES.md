# Dependencies used by ShellPort

ShellPort uses many third-party components. Those components are required in order
for ShellPort to function.

A list of used components can be found inside `package.json` and `go.mod` file.

Major dependencies include:

## For front-end application

- [Node.js](https://nodejs.org/), version 24 or newer for local development,
  lockfile generation, and CI parity, Licensed under MIT license
- [npm](https://www.npmjs.com/), bundled with Node.js and used for frontend
  dependency installation, scripts, and lockfile generation
- [Vue](https://vuejs.org), Licensed under MIT license
- [Vite](https://vite.dev/), Licensed under MIT license
- [Vitest](https://vitest.dev/), Licensed under MIT license
- [XTerm.js](https://xtermjs.org/), Licensed under MIT license
- [@xterm/addon-fit](https://github.com/xtermjs/xterm.js), Licensed under MIT license
- [@xterm/addon-unicode11](https://github.com/xtermjs/xterm.js), Licensed under MIT license
- [@xterm/addon-web-links](https://github.com/xtermjs/xterm.js), Licensed under MIT license
- [@xterm/addon-webgl](https://github.com/xtermjs/xterm.js), Licensed under MIT license
- [normalize.css](https://github.com/necolas/normalize.css), Licensed under MIT license
- [Roboto font](https://en.wikipedia.org/wiki/Roboto), Licensed under Apache license
  Packaged by [Christian Hoffmeister](https://github.com/choffmeister/roboto-fontface-bower), Licensed under Apache 2.0
- [iconv-lite](https://github.com/ashtuchkin/iconv-lite), Licensed under MIT license
- [buffer](https://github.com/feross/buffer), Licensed under MIT license
- [stream-browserify](https://github.com/browserify/stream-browserify), Licensed under MIT license
- [process](https://github.com/defunctzombie/node-process), Licensed under MIT license
- [events](https://github.com/browserify/events), Licensed under MIT license
- [string_decoder](https://github.com/nodejs/string_decoder), Licensed under MIT license
- [fontfaceobserver](https://github.com/bramstein/fontfaceobserver), [View license](https://github.com/bramstein/fontfaceobserver/blob/master/LICENSE)
- [JetBrainsMono Nerd Font](https://www.nerdfonts.com/font-downloads), patched from
  [JetBrains Mono](https://github.com/JetBrains/JetBrainsMono) by
  [Nerd Fonts](https://github.com/ryanoasis/nerd-fonts), Licensed under SIL OFL
  1.1. Bundled font assets are stored under `ui/fonts/JetBrainsMonoNerdFont/`;
  see `ui/fonts/JetBrainsMonoNerdFont/OFL.txt`,
  `ui/fonts/JetBrainsMonoNerdFont/README.md`, and
  `ui/fonts/JetBrainsMonoNerdFont/manifest.json` for license, source release,
  and checksum details.

## For back-end application

- [Go programming language](https://golang.org), [View license](https://github.com/golang/go/blob/master/LICENSE)
- `github.com/gorilla/websocket`, Licensed under BSD-2-Clause license
- `github.com/creack/pty`, Licensed under MIT license
- `golang.org/x/net/proxy` [View license](https://github.com/golang/net/blob/master/LICENSE)
- `golang.org/x/crypto`, [View license](https://github.com/golang/crypto/blob/master/LICENSE)
- [Eternal Terminal](https://github.com/MisterTea/EternalTerminal), installed in
  the Docker image as the local `et` client from the upstream Debian package
  repository, Licensed under Apache-2.0
