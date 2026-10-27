import assert from "node:assert/strict";
import { readdir, readFile } from "node:fs/promises";
import { basename, dirname, join, relative } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const STORES_DIRECTORY = dirname(fileURLToPath(import.meta.url));

test("stores use Zustand directly and keep models and config in their layers", async () => {
  const files = await typescriptSources(STORES_DIRECTORY);

  for (const file of files) {
    if (file.endsWith(".test.ts")) continue;
    const source = await readFile(file, "utf8");
    assertStoreLayerBoundaries(file, source);
    if (!basename(file).endsWith("Store.ts")) continue;

    assert.match(
      source,
      /from ["']zustand\/vanilla["']/,
      `${relative(STORES_DIRECTORY, file)} must import Zustand's vanilla store`,
    );
    assert.match(
      source,
      /\bcreateStore(?:\s*<|\s*\()/,
      `${relative(STORES_DIRECTORY, file)} must use Zustand createStore`,
    );
  }
});

function assertStoreLayerBoundaries(file: string, source: string): void {
  const module = ts.createSourceFile(file, source, ts.ScriptTarget.Latest);
  const path = relative(STORES_DIRECTORY, file);

  function visit(node: ts.Node): void {
    const line = module.getLineAndCharacterOfPosition(node.getStart(module)).line + 1;
    assert.ok(
      !ts.isInterfaceDeclaration(node)
        && !ts.isTypeAliasDeclaration(node)
        && !ts.isEnumDeclaration(node)
        && !ts.isTypeLiteralNode(node),
      `${path}:${line} must declare models and contracts in models/`,
    );
    ts.forEachChild(node, visit);
  }
  visit(module);

  for (const statement of module.statements) {
    if (!ts.isVariableStatement(statement)) continue;
    if (!(statement.declarationList.flags & ts.NodeFlags.Const)) continue;
    for (const declaration of statement.declarationList.declarations) {
      if (!ts.isIdentifier(declaration.name)) continue;
      assert.doesNotMatch(
        declaration.name.text,
        /^[A-Z][A-Z0-9_]*$/,
        `${path} must declare named fixed defaults and settings in config/`,
      );
    }
  }
}

async function typescriptSources(directory: string): Promise<string[]> {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = await Promise.all(entries.map(async (entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return typescriptSources(path);
    return entry.isFile() && entry.name.endsWith(".ts") ? [path] : [];
  }));
  return files.flat().sort();
}
