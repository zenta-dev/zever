import * as vscode from 'vscode';
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
  TransportKind,
} from 'vscode-languageclient/node';

let client: LanguageClient | undefined;

export function activate(_context: vscode.ExtensionContext): void {
  const serverOptions: ServerOptions = {
    command: 'zever-lsp',
    transport: TransportKind.stdio,
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: 'file', language: 'zen' }],
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
  client.start().catch((err: unknown) => {
    const detail = err instanceof Error ? err.message : String(err);
    vscode.window.showErrorMessage(
      `Failed to start zever-lsp: ${detail}. Install it with: ` +
        'git clone https://github.com/zenta-dev/zever.git && ' +
        'go install ./tools/zever-lsp (run inside the clone).'
    );
  });

  _context.subscriptions.push({ dispose: () => client?.stop() });
}

export function deactivate(): Thenable<void> | undefined {
  return client?.stop();
}
