// Minimal `vscode` API stub for headless `npm test` runs.
const messages = [];
const warnings = [];
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
  showWarningMessage: (message) => {
    warnings.push(message);
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

// Extension lookup knob for version-check tests: `__version` is the
// `packageJSON.version` reported for any extension id (undefined mimics an
// unresolvable host), and `__requestedIds` records lookups so tests can
// assert the exact `publisher.name` id the client uses.
const extensions = {
  __requestedIds: [],
  __version: undefined,
  getExtension: (id) => {
    extensions.__requestedIds.push(id);
    if (extensions.__version === undefined) {
      return undefined;
    }
    return { packageJSON: { version: extensions.__version } };
  },
};

module.exports = {
  window,
  workspace,
  commands,
  languages,
  extensions,
  DiagnosticSeverity,
  LanguageStatusSeverity,
  __messages: messages,
  __warnings: warnings,
  __statusItems: statusItems,
  __commands: registeredCommands,
  __commandHandlers: commandHandlers,
};
