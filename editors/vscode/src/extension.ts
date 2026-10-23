import * as vscode from 'vscode';
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
  TransportKind,
} from 'vscode-languageclient/node';

let client: LanguageClient | undefined;

// ZenDeclCounts tallies the top-level schema declarations in one `.zen`
// document. It is scanned from the open document text client-side, so the
// status item needs no extra server round-trip.
export interface ZenDeclCounts {
  entities: number;
  messages: number;
  services: number;
}

const declPattern = /^\s*(entity|message|service)\s+[A-Za-z_]/;

// countDecls counts top-level `entity`/`message`/`service` declarations.
// Matching anchors at line start (after optional indentation), so `//`
// comment lines never match. Kept pure for headless unit tests.
export function countDecls(text: string): ZenDeclCounts {
  const counts: ZenDeclCounts = { entities: 0, messages: 0, services: 0 };
  for (const line of text.split('\n')) {
    const match = declPattern.exec(line);
    if (match === null) {
      continue;
    }
    switch (match[1]) {
      case 'entity':
        counts.entities += 1;
        break;
      case 'message':
        counts.messages += 1;
        break;
      case 'service':
        counts.services += 1;
        break;
    }
  }
  return counts;
}

function pluralize(count: number, singular: string, plural: string): string {
  return `${count} ${count === 1 ? singular : plural}`;
}

// errorCount tallies error-severity diagnostics on the document. A missing
// severity defaults to Error in VS Code, so those count too. Any failure
// reading diagnostics degrades to zero rather than breaking activation.
function errorCount(doc: vscode.TextDocument): number {
  let diagnostics: readonly vscode.Diagnostic[];
  try {
    diagnostics = vscode.languages.getDiagnostics(doc.uri);
  } catch {
    return 0;
  }
  return diagnostics.filter(
    (d) => d.severity === undefined || d.severity === vscode.DiagnosticSeverity.Error
  ).length;
}

function refreshStatus(item: vscode.LanguageStatusItem): void {
  const doc = vscode.window.activeTextEditor?.document;
  if (doc === undefined || doc.languageId !== 'zen') {
    item.text = '$(database) Zen';
    item.detail = 'Open a .zen file to see schema stats';
    item.severity = vscode.LanguageStatusSeverity.Information;
    return;
  }
  const counts = countDecls(doc.getText());
  const errors = errorCount(doc);
  item.text =
    `$(database) Zen: ${pluralize(counts.entities, 'entity', 'entities')}, ` +
    `${pluralize(counts.messages, 'message', 'messages')}, ` +
    `${pluralize(counts.services, 'service', 'services')}, ` +
    `${pluralize(errors, 'error', 'errors')}`;
  item.detail = errors === 0 ? 'zen schema' : 'zen schema has errors — see the Problems panel';
  item.severity =
    errors === 0
      ? vscode.LanguageStatusSeverity.Information
      : vscode.LanguageStatusSeverity.Warning;
}

function serverCommand(): string {
  try {
    const cfg = vscode.workspace.getConfiguration('zever');
    const value = cfg.get<string>('serverPath');
    return value && value.trim() !== '' ? value : 'zever-lsp';
  } catch {
    return 'zever-lsp';
  }
}

// parseMajorMinor extracts the [major, minor] pair from a version string,
// tolerating a leading `v` and trailing patch/prerelease segments. It
// returns undefined when no leading numeric pair is present.
function parseMajorMinor(version: string): [number, number] | undefined {
  if (!version) {
    return undefined;
  }
  const match = /^\s*v?(\d+)\.(\d+)/.exec(version);
  if (match === null) {
    return undefined;
  }
  return [Number(match[1]), Number(match[2])];
}

// isCompatibleVersion reports whether a language server version is
// compatible with the extension version. Compatibility means equal major
// and minor numbers; patch differences are fine. Missing or unparseable
// versions never trigger a warning, so this returns true for them. Kept
// pure for headless unit tests.
export function isCompatibleVersion(server: string, ext: string): boolean {
  const serverPair = parseMajorMinor(server);
  const extPair = parseMajorMinor(ext);
  if (serverPair === undefined || extPair === undefined) {
    return true;
  }
  return serverPair[0] === extPair[0] && serverPair[1] === extPair[1];
}

// extensionVersion returns this extension's own version from its package.json
// contribution, or undefined when unavailable (dev hosts, headless tests).
// Never throws.
function extensionVersion(): string | undefined {
  try {
    const ext = vscode.extensions.getExtension('zenta-dev.zever-dsl');
    const version: unknown = ext?.packageJSON?.version;
    return typeof version === 'string' && version !== '' ? version : undefined;
  } catch {
    return undefined;
  }
}

// checkServerVersion warns once when the running language server's
// major.minor version differs from the extension's own version, hinting at
// a stale `zever-lsp` binary. Missing versions on either side stay silent.
function checkServerVersion(running: LanguageClient): void {
  let serverVersion: string | undefined;
  try {
    serverVersion = running.initializeResult?.serverInfo?.version;
  } catch {
    return;
  }
  if (typeof serverVersion !== 'string' || serverVersion === '') {
    return;
  }
  const extVersion = extensionVersion();
  if (extVersion === undefined) {
    return;
  }
  if (!isCompatibleVersion(serverVersion, extVersion)) {
    try {
      vscode.window.showWarningMessage(
        `Zever: language server version ${serverVersion} differs from extension version ${extVersion}; ` +
          'update zever-lsp to match the extension (see the README install steps).'
      );
    } catch {
      // Notification-only; never break activation.
    }
  }
}

function startClient(): void {
  const serverOptions: ServerOptions = {
    command: serverCommand(),
    transport: TransportKind.stdio,
  };

  const clientOptions: LanguageClientOptions = {
    // `untitled` covers unsaved `zen` editors: the client forwards their
    // requests, but the server keys everything by filesystem path and
    // answers unknown documents with empty results (no errors), so an
    // untitled tab gets grammar highlighting only until it is saved.
    documentSelector: [
      { scheme: 'file', language: 'zen' },
      { scheme: 'untitled', language: 'zen' },
    ],
  };

  client = new LanguageClient(
    'zeverLanguageServer',
    'Zever Language Server',
    serverOptions,
    clientOptions
  );

  // Fire-and-forget start: a missing binary surfaces one error notification
  // with the install hint. No retry/respawn loop by design — VS Code
  // re-invokes activate on the next `zen` document, which retries naturally.
  // On success the server version is compared once against the extension
  // version (warns on major.minor mismatch, silent otherwise).
  client
    .start()
    .then(() => {
      if (client !== undefined) {
        checkServerVersion(client);
      }
    })
    .catch((err: unknown) => {
    const detail = err instanceof Error ? err.message : String(err);
    vscode.window.showErrorMessage(
      `Failed to start zever-lsp: ${detail}. Install it with: ` +
        'git clone https://github.com/zenta-dev/zever.git && ' +
        'go install ./tools/zever-lsp (run inside the clone).'
    );
  });
}

// executeCommandForActiveEditor sends a workspace/executeCommand request for
// the active zen document's URI. It no-ops when no zen editor is active or
// the client is not running, so palette invocations from other languages
// never surface an error.
async function executeCommandForActiveEditor(command: string): Promise<void> {
  const doc = vscode.window.activeTextEditor?.document;
  if (doc === undefined || doc.languageId !== 'zen' || client === undefined) {
    return;
  }
  await client.sendRequest('workspace/executeCommand', {
    command,
    arguments: [doc.uri.toString()],
  });
}

export function activate(context: vscode.ExtensionContext): void {
  startClient();

  const status = vscode.languages.createLanguageStatusItem('zever.status', { language: 'zen' });
  status.name = 'Zever schema stats';
  refreshStatus(status);
  const refresh = (): void => {
    refreshStatus(status);
  };

  const restartDisposable = vscode.commands.registerCommand(
    'zever.restartServer',
    async () => {
      if (client) {
        await client.stop();
      }
      startClient();
      vscode.window.showInformationMessage('Zever: Language server restarted.');
    }
  );

  const validateDisposable = vscode.commands.registerCommand('zever.validate', () =>
    executeCommandForActiveEditor('zever.validate')
  );

  const fixAllDisposable = vscode.commands.registerCommand('zever.fixAll', () =>
    executeCommandForActiveEditor('zever.fixAll')
  );

  context.subscriptions.push(
    status,
    vscode.workspace.onDidChangeTextDocument((e) => {
      if (e.document.languageId === 'zen') {
        refresh();
      }
    }),
    vscode.workspace.onDidOpenTextDocument((doc) => {
      if (doc.languageId === 'zen') {
        refresh();
      }
    }),
    vscode.window.onDidChangeActiveTextEditor(refresh),
    vscode.languages.onDidChangeDiagnostics(refresh)
  );

  context.subscriptions.push({
    dispose: () => {
      client?.stop();
      restartDisposable.dispose();
      validateDisposable.dispose();
      fixAllDisposable.dispose();
    },
  });
}

export function deactivate(): Thenable<void> | undefined {
  return client?.stop();
}
