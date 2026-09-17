// Minimal `vscode-languageclient/node` stub for headless `npm test` runs.
// Mirrors the constructor/start/stop surface extension.ts uses, with
// controllable start failures. Resolved via NODE_PATH=./test/stubs.

const TransportKind = { stdio: 0, ipc: 1, pipe: 2, socket: 3 };

class LanguageClient {
  static instances = [];
  static failWith = undefined;

  constructor(id, name, serverOptions, clientOptions) {
    this.id = id;
    this.name = name;
    this.serverOptions = serverOptions;
    this.clientOptions = clientOptions;
    this.stopped = false;
    LanguageClient.instances.push(this);
  }

  start() {
    if (LanguageClient.failWith !== undefined) {
      return Promise.reject(LanguageClient.failWith);
    }
    return Promise.resolve();
  }

  stop() {
    this.stopped = true;
    return Promise.resolve();
  }
}

module.exports = { LanguageClient, TransportKind };
