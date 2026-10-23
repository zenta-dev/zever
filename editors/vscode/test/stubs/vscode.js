// Minimal `vscode` API stub for headless `npm test` runs.
const messages = [];

const window = {
  showErrorMessage: (message) => {
    messages.push(message);
    return Promise.resolve(undefined);
  },
  showInformationMessage: (message) => {
    return Promise.resolve(undefined);
  },
};

const workspace = {
  getConfiguration: (_section) => ({
    get: (_key, defaultValue) => defaultValue,
  }),
};

const commands = {
  registerCommand: (_command, _callback) => ({ dispose: () => {} }),
};

module.exports = { window, workspace, commands, __messages: messages };
