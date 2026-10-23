// Headless unit tests for the extension host entry points.
// Run with `npm test` (tsc + node --test). `vscode` and
// `vscode-languageclient/node` are stubbed via a Module._load hook scoped to
// this test file and extension.js only, so the real dependency trees (if ever
// loaded) keep resolving the genuine modules.
// They cover: the activate error path (missing zever-lsp binary surfaces one
// install-hint notification, leaking no paths), the happy path (no error),
// deactivate with/without a live client, the countDecls schema-stats helper,
// the zen language status item, and the palette commands (validate/fixAll
// forward the active editor's URI over workspace/executeCommand).
import { strict as assert } from 'node:assert';
import { describe, it, beforeEach } from 'node:test';
import * as path from 'node:path';
import Module from 'node:module';
import type * as VscodeApi from 'vscode';
import type { LanguageClient as LanguageClientType } from 'vscode-languageclient/node';
import type * as ExtensionModule from './extension';

interface StatusItemStub {
  id: string;
  selector: unknown;
  text: string;
}

interface VscodeStub {
  window: {
    showErrorMessage: (message: string) => Thenable<string | undefined>;
    activeTextEditor?: { document: VscodeApi.TextDocument };
  };
  languages: {
    createLanguageStatusItem: (id: string, selector: unknown) => StatusItemStub;
    getDiagnostics: (uri: VscodeApi.Uri) => VscodeApi.Diagnostic[];
  };
  __messages: string[];
  __statusItems: StatusItemStub[];
  __commands: string[];
  __commandHandlers: Record<string, () => Promise<unknown> | unknown>;
}

interface ServerOptionsStub {
  command: string;
  transport: number;
}

interface SentRequest {
  method: string;
  params: unknown;
}

interface LanguageClientStub {
  id: string;
  name: string;
  serverOptions: ServerOptionsStub;
  clientOptions: { documentSelector: unknown };
  stopped: boolean;
  sentRequests: SentRequest[];
  start: () => Promise<void>;
  stop: () => Promise<void>;
  sendRequest: (method: string, params: unknown) => Promise<unknown>;
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
const { activate, deactivate, countDecls } = require('./extension') as typeof ExtensionModule;

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

  it('selects both saved and untitled zen documents', () => {
    activate(makeContext());

    assert.equal(clientCtor.instances.length, 1);
    const [instance] = clientCtor.instances;
    assert.deepEqual(instance.clientOptions.documentSelector, [
      { scheme: 'file', language: 'zen' },
      { scheme: 'untitled', language: 'zen' },
    ]);
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

  it('registers subscriptions that stop the client', () => {
    const raw: TestContext = { subscriptions: [] };
    activate(raw as unknown as VscodeApi.ExtensionContext);

    assert.ok(raw.subscriptions.length >= 1);
    for (const sub of raw.subscriptions) {
      void sub.dispose();
    }
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

describe('countDecls', () => {
  it('counts each declaration kind', () => {
    assert.deepEqual(countDecls('entity A {\n}\nmessage M {\n}\nservice S {\n}\n'), {
      entities: 1,
      messages: 1,
      services: 1,
    });
  });

  it('counts indented declarations and multiples of one kind', () => {
    assert.deepEqual(countDecls('  entity A {\n}\n\tentity B {\n}\n'), {
      entities: 2,
      messages: 0,
      services: 0,
    });
  });

  it('ignores // comment lines', () => {
    assert.deepEqual(countDecls('// entity Fake {\nentity Real {\n}\n'), {
      entities: 1,
      messages: 0,
      services: 0,
    });
  });

  it('requires a name after the keyword', () => {
    assert.deepEqual(countDecls('entity\nentity \nmessage\nservice\n'), {
      entities: 0,
      messages: 0,
      services: 0,
    });
  });

  it('does not count keywords mid-line', () => {
    assert.deepEqual(countDecls('foo entity Bar\n'), {
      entities: 0,
      messages: 0,
      services: 0,
    });
  });

  it('returns zeros for empty text', () => {
    assert.deepEqual(countDecls(''), { entities: 0, messages: 0, services: 0 });
  });
});

describe('status item', () => {
  const defaultGetDiagnostics = vscodeStub.languages.getDiagnostics;

  function fakeZenDoc(text: string): VscodeApi.TextDocument {
    return {
      languageId: 'zen',
      getText: () => text,
      uri: {} as VscodeApi.Uri,
    } as unknown as VscodeApi.TextDocument;
  }

  function setActiveZenEditor(text: string | undefined): void {
    vscodeStub.window.activeTextEditor =
      text === undefined ? undefined : { document: fakeZenDoc(text) };
  }

  beforeEach(() => {
    vscodeStub.__messages.length = 0;
    vscodeStub.__statusItems.length = 0;
    clientCtor.instances.length = 0;
    clientCtor.failWith = undefined;
    setActiveZenEditor(undefined);
    vscodeStub.languages.getDiagnostics = defaultGetDiagnostics;
  });

  it('creates a zen language status item', () => {
    activate(makeContext());

    assert.equal(vscodeStub.__statusItems.length, 1);
    const [item] = vscodeStub.__statusItems;
    assert.equal(item.id, 'zever.status');
    assert.deepEqual(item.selector, { language: 'zen' });
  });

  it('shows idle text with no active zen editor', () => {
    activate(makeContext());

    assert.equal(vscodeStub.__statusItems[0].text, '$(database) Zen');
  });

  it('shows declaration counts for the active zen document', () => {
    setActiveZenEditor('entity A {\n}\nentity B {\n}\nmessage M {\n}\nservice S {\n}\n');
    activate(makeContext());

    assert.equal(
      vscodeStub.__statusItems[0].text,
      '$(database) Zen: 2 entities, 1 message, 1 service, 0 errors'
    );
  });

  it('uses singular nouns for counts of one', () => {
    setActiveZenEditor('entity A {\n}\n');
    activate(makeContext());

    assert.equal(
      vscodeStub.__statusItems[0].text,
      '$(database) Zen: 1 entity, 0 messages, 0 services, 0 errors'
    );
  });

  it('includes the diagnostic error count, ignoring warnings', () => {
    setActiveZenEditor('entity A {\n}\n');
    vscodeStub.languages.getDiagnostics = () => [
      { severity: vscode.DiagnosticSeverity.Error } as VscodeApi.Diagnostic,
      { severity: vscode.DiagnosticSeverity.Error } as VscodeApi.Diagnostic,
      { severity: vscode.DiagnosticSeverity.Warning } as VscodeApi.Diagnostic,
    ];
    activate(makeContext());

    assert.match(vscodeStub.__statusItems[0].text, /2 errors/);
  });

  it('ignores active editors for other languages', () => {
    vscodeStub.window.activeTextEditor = {
      document: {
        languageId: 'typescript',
        getText: () => 'entity Fake {\n}\n',
      } as unknown as VscodeApi.TextDocument,
    };
    activate(makeContext());

    assert.equal(vscodeStub.__statusItems[0].text, '$(database) Zen');
  });
});

describe('palette commands', () => {
  beforeEach(() => {
    vscodeStub.__messages.length = 0;
    vscodeStub.__commands.length = 0;
    for (const key of Object.keys(vscodeStub.__commandHandlers)) {
      delete vscodeStub.__commandHandlers[key];
    }
    clientCtor.instances.length = 0;
    clientCtor.failWith = undefined;
    vscodeStub.window.activeTextEditor = undefined;
  });

  function setActiveZenDocument(uri: string): void {
    vscodeStub.window.activeTextEditor = {
      document: {
        languageId: 'zen',
        getText: () => 'entity A {\n}\n',
        uri: { toString: () => uri },
      } as unknown as VscodeApi.TextDocument,
    };
  }

  it('registers validate and fixAll alongside restart', () => {
    activate(makeContext());

    assert.ok(vscodeStub.__commands.includes('zever.restartServer'));
    assert.ok(vscodeStub.__commands.includes('zever.validate'));
    assert.ok(vscodeStub.__commands.includes('zever.fixAll'));
  });

  it('validate sends workspace/executeCommand for the active editor URI', async () => {
    setActiveZenDocument('file:///a.zen');
    activate(makeContext());

    await vscodeStub.__commandHandlers['zever.validate']();

    const [instance] = clientCtor.instances;
    assert.equal(instance.sentRequests.length, 1);
    assert.deepEqual(instance.sentRequests[0], {
      method: 'workspace/executeCommand',
      params: { command: 'zever.validate', arguments: ['file:///a.zen'] },
    });
  });

  it('fixAll sends workspace/executeCommand for the active editor URI', async () => {
    setActiveZenDocument('file:///b.zen');
    activate(makeContext());

    await vscodeStub.__commandHandlers['zever.fixAll']();

    const [instance] = clientCtor.instances;
    assert.equal(instance.sentRequests.length, 1);
    assert.deepEqual(instance.sentRequests[0], {
      method: 'workspace/executeCommand',
      params: { command: 'zever.fixAll', arguments: ['file:///b.zen'] },
    });
  });

  it('sends nothing with no active editor', async () => {
    activate(makeContext());

    await vscodeStub.__commandHandlers['zever.validate']();
    await vscodeStub.__commandHandlers['zever.fixAll']();

    const [instance] = clientCtor.instances;
    assert.equal(instance.sentRequests.length, 0);
  });

  it('sends nothing for a non-zen document', async () => {
    vscodeStub.window.activeTextEditor = {
      document: {
        languageId: 'typescript',
        getText: () => 'const x = 1;',
        uri: { toString: () => 'file:///c.ts' },
      } as unknown as VscodeApi.TextDocument,
    };
    activate(makeContext());

    await vscodeStub.__commandHandlers['zever.validate']();

    const [instance] = clientCtor.instances;
    assert.equal(instance.sentRequests.length, 0);
  });
});
