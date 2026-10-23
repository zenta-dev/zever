// Minimal `vscode` API stub for headless `npm test` runs.
const messages = [];
const statusItems = [];
const registeredCommands = [];
const commandHandlers = {};

const window = {
  showErrorMessage: (message) => {
    messages.push(message);
    return Promise.resolve(undefined);
  },
  showInformationMessage: (message) => {
    return Promise.resolve(undefined);
  },
  activeTextEditor: undefined,
  onDidChangeActiveTextEditor: (_listener) => ({ dispose: () => {} }),
};

const workspace = {
  getConfiguration: (_section) => ({
    get: (_key, defaultValue) => defaultValue,
  }),
  onDidChangeTextDocument: (_listener) => ({ dispose: () => {} }),
  onDidOpenTextDocument: (_listener) => ({ dispose: () => {} }),
};

const commands = {
  registerCommand: (command, callback) => {
    registeredCommands.push(command);
    commandHandlers[command] = callback;
    return { dispose: () => {} };
  },
};

const languages = {
  createLanguageStatusItem: (id, selector) => {
    const item = {
      id,
      selector,
      name: undefined,
      text: '',
      detail: undefined,
      severity: 0,
      dispose: () => {},
    };
    statusItems.push(item);
    return item;
  },
  getDiagnostics: (_uri) => [],
  onDidChangeDiagnostics: (_listener) => ({ dispose: () => {} }),
};

const DiagnosticSeverity = { Error: 0, Warning: 1, Information: 2, Hint: 3 };
const LanguageStatusSeverity = { Information: 0, Warning: 1, Error: 2 };

module.exports = {
  window,
  workspace,
  commands,
  languages,
  DiagnosticSeverity,
  LanguageStatusSeverity,
  __messages: messages,
  __statusItems: statusItems,
  __commands: registeredCommands,
  __commandHandlers: commandHandlers,
};
