// Headless unit tests for the extension host entry points.
// Run with `npm test` (tsc + node --test). `vscode` and
// `vscode-languageclient/node` are stubbed via a Module._load hook scoped to
// this test file and extension.js only, so the real dependency trees (if ever
// loaded) keep resolving the genuine modules.
// They cover: the activate error path (missing zever-lsp binary surfaces one
// install-hint notification, leaking no paths), the happy path (no error),
// and deactivate with/without a live client.
import { strict as assert } from 'node:assert';
import { describe, it, beforeEach } from 'node:test';
import * as path from 'node:path';
import Module from 'node:module';
import type * as VscodeApi from 'vscode';
import type { LanguageClient as LanguageClientType } from 'vscode-languageclient/node';
import type * as ExtensionModule from './extension';

interface VscodeStub {
  window: {
    showErrorMessage: (message: string) => Thenable<string | undefined>;
  };
  __messages: string[];
}

interface ServerOptionsStub {
  command: string;
  transport: number;
}

interface LanguageClientStub {
  id: string;
  name: string;
  serverOptions: ServerOptionsStub;
  stopped: boolean;
  start: () => Promise<void>;
  stop: () => Promise<void>;
}

interface LanguageClientCtor {
  instances: LanguageClientStub[];
  failWith: unknown;
  new (
    id: string,
    name: string,
    serverOptions: ServerOptionsStub,
    clientOptions: unknown
  ): LanguageClientStub;
}

type LoadFn = (
  request: string,
  parent?: { filename?: string },
  isMain?: boolean
) => unknown;

const moduleApi = Module as unknown as { _load: LoadFn };
const originalLoad = moduleApi._load.bind(moduleApi);

const stubDir = path.join(__dirname, '..', 'test', 'stubs');
const stubTargets: Readonly<Record<string, string>> = {
  vscode: path.join(stubDir, 'vscode.js'),
  'vscode-languageclient/node': path.join(
    stubDir,
    'vscode-languageclient',
    'node.js'
  ),
};

function isStubbedParent(parent?: { filename?: string }): boolean {
  const filename = parent?.filename ?? '';
  return (
    filename.endsWith(`${path.sep}out${path.sep}extension.js`) ||
    filename.endsWith(`${path.sep}out${path.sep}extension.test.js`)
  );
}

moduleApi._load = function (
  request: string,
  parent?: { filename?: string },
  isMain?: boolean
): unknown {
  const stubPath = stubTargets[request];
  if (stubPath !== undefined && isStubbedParent(parent)) {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    return require(stubPath);
  }
  return originalLoad(request, parent, isMain);
};

// Static imports of 'vscode' / 'vscode-languageclient/node' would resolve
// before the hook below is installed, so values load lazily via require.
// eslint-disable-next-line @typescript-eslint/no-require-imports
const vscode = require(stubTargets['vscode']) as typeof VscodeApi;
// eslint-disable-next-line @typescript-eslint/no-require-imports
const { LanguageClient } = require(stubTargets['vscode-languageclient/node']) as {
  LanguageClient: typeof LanguageClientType;
};
// Imported lazily so the hook above is installed before extension.js loads.
// eslint-disable-next-line @typescript-eslint/no-require-imports
const { activate, deactivate } = require('./extension') as typeof ExtensionModule;

const vscodeStub = vscode as unknown as VscodeStub;
const clientCtor = LanguageClient as unknown as LanguageClientCtor;

interface TestContext {
  subscriptions: Array<{ dispose: () => unknown }>;
}

function makeContext(): VscodeApi.ExtensionContext {
  const context: TestContext = { subscriptions: [] };
  return context as unknown as VscodeApi.ExtensionContext;
}

function flushMicrotasks(): Promise<void> {
  return new Promise((resolve) => setImmediate(resolve));
}

describe('activate', () => {
  beforeEach(() => {
    vscodeStub.__messages.length = 0;
    clientCtor.instances.length = 0;
    clientCtor.failWith = undefined;
  });

  it('starts zever-lsp over stdio with the zen document selector', () => {
    activate(makeContext());

    assert.equal(clientCtor.instances.length, 1);
    const [instance] = clientCtor.instances;
    assert.equal(instance.id, 'zeverLanguageServer');
    assert.equal(instance.name, 'Zever Language Server');
    assert.equal(instance.serverOptions.command, 'zever-lsp');
  });

  it('shows one install hint and no paths when the server fails to start', async () => {
    clientCtor.failWith = new Error('spawn zever-lsp ENOENT');
    activate(makeContext());
    await flushMicrotasks();

    assert.equal(vscodeStub.__messages.length, 1);
    const [message] = vscodeStub.__messages;
    assert.match(message, /Failed to start zever-lsp/);
    assert.match(message, /go install \.\/tools\/zever-lsp/);
    assert.match(message, /zenta-dev\/zever/);
    // Secure: the notification carries the static hint only — no absolute
    // paths, no $PATH dumps beyond err.message.
    assert.doesNotMatch(message, /\/home\//);
    assert.doesNotMatch(message, /\/Users\//);
    assert.doesNotMatch(message, /C:\\/);
  });

  it('renders non-Error rejections without throwing', async () => {
    clientCtor.failWith = 'string rejection';
    activate(makeContext());
    await flushMicrotasks();

    assert.equal(vscodeStub.__messages.length, 1);
    assert.match(vscodeStub.__messages[0], /string rejection/);
  });

  it('shows no error when the server starts cleanly', async () => {
    activate(makeContext());
    await flushMicrotasks();

    assert.equal(vscodeStub.__messages.length, 0);
  });

  it('registers a subscription that stops the client', () => {
    const raw: TestContext = { subscriptions: [] };
    activate(raw as unknown as VscodeApi.ExtensionContext);

    assert.equal(raw.subscriptions.length, 1);
    void raw.subscriptions[0].dispose();
    assert.equal(clientCtor.instances[0].stopped, true);
  });
});

describe('deactivate', () => {
  beforeEach(() => {
    vscodeStub.__messages.length = 0;
    clientCtor.instances.length = 0;
    clientCtor.failWith = undefined;
  });

  it('stops the active client and resolves', async () => {
    activate(makeContext());

    const result = deactivate();
    assert.ok(result instanceof Promise);
    await result;
    assert.equal(clientCtor.instances[0].stopped, true);
  });
});
