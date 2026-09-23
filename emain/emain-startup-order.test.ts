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

function isAwaitedGuardThatReturns(stmt: ts.Statement, guardName: string): boolean {
    if (!ts.isIfStatement(stmt)) {
        return false;
    }
    const cond = unwrapParens(stmt.expression);
    if (!ts.isPrefixUnaryExpression(cond) || cond.operator !== ts.SyntaxKind.ExclamationToken) {
        return false;
    }
    const operand = unwrapParens(cond.operand);
    if (!ts.isAwaitExpression(operand) || !isCallTo(unwrapParens(operand.expression), guardName)) {
        return false;
    }
    return containsNode(stmt.thenStatement, ts.isReturnStatement);
}

function parseEmain(): ts.SourceFile {
    return ts.createSourceFile(EmainPath, fs.readFileSync(EmainPath, "utf8"), ts.ScriptTarget.Latest, true);
}

function appMainStatements(): ts.NodeArray<ts.Statement> {
    const appMain = parseEmain().statements.find(
        (s): s is ts.FunctionDeclaration => ts.isFunctionDeclaration(s) && s.name?.text === "appMain"
    );
    expect(appMain?.body).toBeDefined();
    return appMain.body.statements;
}

function importsFromPlatform(name: string): boolean {
    return parseEmain().statements.some(
        (s) =>
            ts.isImportDeclaration(s) &&
            ts.isStringLiteral(s.moduleSpecifier) &&
            s.moduleSpecifier.text === "./emain-platform" &&
            s.importClause?.namedBindings != null &&
            ts.isNamedImports(s.importClause.namedBindings) &&
            s.importClause.namedBindings.elements.some((e) => e.name.text === name)
    );
}

describe("emain.ts startup order", () => {
    it("awaits resolveLegacyInstanceBlock() and returns on false before starting the server", () => {
        const stmts = appMainStatements();
        const guardIdx = stmts.findIndex((s) => isAwaitedGuardThatReturns(s, "resolveLegacyInstanceBlock"));
        const srvIdx = stmts.findIndex((s) => containsNode(s, (n) => isCallTo(n, "runRemoteTermSrv")));
        expect(
            guardIdx,
            "no `if (!(await resolveLegacyInstanceBlock())) { ...; return; }` in appMain"
        ).toBeGreaterThanOrEqual(0);
        expect(srvIdx, "no runRemoteTermSrv() call in appMain").toBeGreaterThanOrEqual(0);
        expect(guardIdx).toBeLessThan(srvIdx);
    });

    // After the legacy-instance guard: "Migrate anyway" runs the migration again inside it.
    it("awaits resolveIncompleteMigrationBlock() and returns on false after the legacy guard, before the server", () => {
        const stmts = appMainStatements();
        const legacyIdx = stmts.findIndex((s) => isAwaitedGuardThatReturns(s, "resolveLegacyInstanceBlock"));
        const guardIdx = stmts.findIndex((s) => isAwaitedGuardThatReturns(s, "resolveIncompleteMigrationBlock"));
        const srvIdx = stmts.findIndex((s) => containsNode(s, (n) => isCallTo(n, "runRemoteTermSrv")));
        expect(
            guardIdx,
            "no `if (!(await resolveIncompleteMigrationBlock())) { ...; return; }` in appMain"
        ).toBeGreaterThanOrEqual(0);
        expect(guardIdx).toBeGreaterThan(legacyIdx);
        expect(guardIdx).toBeLessThan(srvIdx);
    });

    it.each([["resolveLegacyInstanceBlock"], ["resolveIncompleteMigrationBlock"]])(
        "imports %s from emain-platform",
        (name) => {
            expect(importsFromPlatform(name)).toBe(true);
        }
    );
});
