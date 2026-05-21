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
- [Hack Font](https://github.com/source-foundry/Hack), [View license](https://github.com/source-foundry/Hack/blob/master/LICENSE.md)
- [Nerd Fonts](https://www.nerdfonts.com/), packaged by [@azurity/pure-nerd-font](http://github.com/azurity/pure-nerd-font)
  includes icons from following fonts:
  - [Powerline Extra Symbols](https://github.com/ryanoasis/powerline-extra-symbols), Licensed under MIT license
  - [Font Awesome](https://github.com/FortAwesome/Font-Awesome), [View license](https://github.com/FortAwesome/Font-Awesome/blob/6.x/LICENSE.txt)
  - [Font Awesome Extension](https://github.com/AndreLZGava/font-awesome-extension), Licensed under MIT license
  - [Material Design Icons](https://github.com/Templarian/MaterialDesign), [View license](https://github.com/Templarian/MaterialDesign/blob/master/LICENSE)
  - [Weather Icons](https://github.com/erikflowers/weather-icons), Licensed under SIL OFL 1.1
  - [Devicons](https://github.com/vorillaz/devicons), Licensed under MIT license
  - [Octicons](https://github.com/primer/octicons), Licensed under MIT license
  - [Codicons](https://github.com/microsoft/vscode-codicons), Licensed under MIT License
  - [Font Logos (Formerly Font Linux)](https://github.com/Lukas-W/font-logos), Licensed under Unlicense license
  - [Pomicons](https://github.com/gabrielelana/pomicons), Licensed under OFL-1.1 license
  - ... and more, see [full list](https://github.com/ryanoasis/nerd-fonts/tree/master/src/glyphs)

## For back-end application

- [Go programming language](https://golang.org), [View license](https://github.com/golang/go/blob/master/LICENSE)
- `github.com/gorilla/websocket`, Licensed under BSD-2-Clause license
- `github.com/creack/pty`, Licensed under MIT license
- `golang.org/x/net/proxy` [View license](https://github.com/golang/net/blob/master/LICENSE)
- `golang.org/x/crypto`, [View license](https://github.com/golang/crypto/blob/master/LICENSE)
- [Eternal Terminal](https://github.com/MisterTea/EternalTerminal), installed in
  the Docker image as the local `et` client from the upstream Debian package
  repository, Licensed under Apache-2.0
