#!/usr/bin/env node
// API-list guard for @tingly/projects.
//
// The server is Go, and a Go nil slice is sent as `null`, not `[]`. A list on
// an entity (a project's `repos`, a diff's `files`) comes from a stored JSON
// column or a builder that may leave it nil, and an older server may still send
// null; the generated types say `Repo[]` regardless, so the compiler cannot see
// the crash. `p.repos.length` took the whole Projects page down that way.
//
// This check finds every read of such a list that goes straight to
// `.length` / `.map(...)` / `.filter(...)` and so on. Write it as
// `(p.repos ?? []).map(...)` or `p.repos?.length` instead. It uses the type
// checker, so a local object with a field of the same name is not a hit.
//
// Only lists of entities are checked. The lists a `*Response` wraps (`threads`,
// `events`, ...) are held non-null by the server's own test
// (TestResponsesOfAnEmptyWorkspaceCarryNoNull). It does not see a list that was
// first pulled out with a destructuring (`const { repos } = p`).
//
//   node scripts/check-api-arrays.mjs     # runs in `pnpm lint`

import { existsSync } from 'node:fs';
import { join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import ts from 'typescript';

const ROOT = fileURLToPath(new URL('..', import.meta.url));
const PACKAGE = join(ROOT, 'packages', 'projects');
const SCHEMA = join(PACKAGE, 'src', 'client', 'schema.d.ts');
// What a list is read with. `.includes`, `.at` and the like are covered too:
// any member access on the list itself throws on null.
const READS = new Set([
  'length', 'map', 'filter', 'some', 'every', 'find', 'findIndex', 'findLast', 'flatMap', 'flat', 'forEach', 'reduce',
  'slice', 'join', 'includes', 'indexOf', 'sort', 'concat', 'at', 'entries', 'keys', 'values',
]);

if (!existsSync(SCHEMA)) {
  console.log('api arrays: skipped (run `pnpm gen:api` to generate the client types)');
  process.exit(0);
}

const configPath = join(PACKAGE, 'tsconfig.json');
const config = ts.getParsedCommandLineOfConfigFile(configPath, {}, { ...ts.sys, onUnRecoverableConfigFileDiagnostic: () => {} });
const program = ts.createProgram(config.fileNames, config.options);
const checker = program.getTypeChecker();

const isTest = (f) => /\.test\.tsx?$/.test(f) || f.includes(`${join('src', 'test')}`);

// The generated schema nests as components.schemas.<Name>.<prop>; the name
// directly under `schemas` is the entity, and `*Response` ones are wrappers.
function entityOf(decl) {
  const names = [];
  for (let n = decl.parent; n; n = n.parent) {
    if (ts.isPropertySignature(n) && ts.isIdentifier(n.name)) names.push(n.name.text);
  }
  const at = names.indexOf('schemas');
  return at > 0 ? names[at - 1] : null;
}

const hits = [];
for (const file of program.getSourceFiles()) {
  if (file.isDeclarationFile || !file.fileName.startsWith(join(PACKAGE, 'src')) || isTest(file.fileName)) continue;
  const visit = (node) => {
    // <owner>.<list>.<read>, without a `?.` before the list or the read.
    if (ts.isPropertyAccessExpression(node) && READS.has(node.name.text) && !node.questionDotToken
      && ts.isPropertyAccessExpression(node.expression) && !node.expression.questionDotToken) {
      const list = node.expression;
      const symbol = checker.getSymbolAtLocation(list.name);
      const decl = symbol?.declarations?.[0];
      if (decl && ts.isPropertySignature(decl) && !decl.questionToken && decl.getSourceFile().fileName === SCHEMA
        && decl.type && ts.isArrayTypeNode(decl.type)) {
        const entity = entityOf(decl);
        if (entity && !entity.endsWith('Response')) {
          const { line } = file.getLineAndCharacterOfPosition(list.name.getStart());
          hits.push(`${relative(ROOT, file.fileName)}:${line + 1}: ${entity}.${list.name.text}.${node.name.text} (use \`(${list.getText()} ?? [])\` or \`?.\`)`);
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(file);
}

if (hits.length) {
  console.error('api arrays: a list of an API entity may be null (a Go nil slice); guard these reads:');
  for (const h of hits) console.error(`  ${h}`);
  process.exit(1);
}
console.log('api arrays: ok');
