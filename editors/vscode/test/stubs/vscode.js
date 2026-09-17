// Minimal `vscode` API stub for headless `npm test` runs.
// Only the surface extension.ts touches is implemented. Resolved via
// NODE_PATH=./test/stubs (see the `test` script in package.json).

/** @type {string[]} */
const messages = [];

const window = {
  showErrorMessage: (message) => {
    messages.push(message);
    return Promise.resolve(undefined);
  },
};

module.exports = { window, __messages: messages };
