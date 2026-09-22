// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import fs from "fs";
import path from "path";
import ts from "typescript";
import { describe, expect, it } from "vitest";

// appMain() cannot be imported (emain.ts starts Electron at module load), so its startup order is
// checked on the parsed source instead.
const EmainPath = path.join(import.meta.dirname, "emain.ts");

function unwrapParens(node: ts.Node): ts.Node {
    while (ts.isParenthesizedExpression(node)) {
        node = node.expression;
    }
    return node;
}

function isCallTo(node: ts.Node, name: string): boolean {
    return ts.isCallExpression(node) && ts.isIdentifier(node.expression) && node.expression.text === name;
}

// Descends into everything except nested functions, whose bodies do not run in this statement.
function containsNode(node: ts.Node, pred: (n: ts.Node) => boolean): boolean {
    if (pred(node)) {
        return true;
    }
    if (ts.isFunctionLike(node)) {
        return false;
    }
    return ts.forEachChild(node, (child) => (containsNode(child, pred) ? true : undefined)) ?? false;
}

function isAwaitedGuardThatReturns(stmt: ts.Statement): boolean {
    if (!ts.isIfStatement(stmt)) {
        return false;
    }
    const cond = unwrapParens(stmt.expression);
    if (!ts.isPrefixUnaryExpression(cond) || cond.operator !== ts.SyntaxKind.ExclamationToken) {
        return false;
    }
    const operand = unwrapParens(cond.operand);
    if (!ts.isAwaitExpression(operand) || !isCallTo(unwrapParens(operand.expression), "resolveLegacyInstanceBlock")) {
        return false;
    }
    return containsNode(stmt.thenStatement, ts.isReturnStatement);
}

function parseEmain(): ts.SourceFile {
    return ts.createSourceFile(EmainPath, fs.readFileSync(EmainPath, "utf8"), ts.ScriptTarget.Latest, true);
}

describe("emain.ts startup order", () => {
    it("awaits resolveLegacyInstanceBlock() and returns on false before starting the server", () => {
        const source = parseEmain();
        const appMain = source.statements.find(
            (s): s is ts.FunctionDeclaration => ts.isFunctionDeclaration(s) && s.name?.text === "appMain"
        );
        expect(appMain?.body).toBeDefined();
        const stmts = appMain.body.statements;
        const guardIdx = stmts.findIndex(isAwaitedGuardThatReturns);
        const srvIdx = stmts.findIndex((s) => containsNode(s, (n) => isCallTo(n, "runRemoteTermSrv")));
        expect(
            guardIdx,
            "no `if (!(await resolveLegacyInstanceBlock())) { ...; return; }` in appMain"
        ).toBeGreaterThanOrEqual(0);
        expect(srvIdx, "no runRemoteTermSrv() call in appMain").toBeGreaterThanOrEqual(0);
        expect(guardIdx).toBeLessThan(srvIdx);
    });

    it("imports resolveLegacyInstanceBlock from emain-platform", () => {
        const source = parseEmain();
        const imported = source.statements.some(
            (s) =>
                ts.isImportDeclaration(s) &&
                ts.isStringLiteral(s.moduleSpecifier) &&
                s.moduleSpecifier.text === "./emain-platform" &&
                s.importClause?.namedBindings != null &&
                ts.isNamedImports(s.importClause.namedBindings) &&
                s.importClause.namedBindings.elements.some((e) => e.name.text === "resolveLegacyInstanceBlock")
        );
        expect(imported).toBe(true);
    });
});
