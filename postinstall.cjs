const { execSync } = require("child_process");

const isCfPages = process.env.CF_PAGES === "1" || process.env.CF_PAGES === "true";
const skip = process.env.REMOTETERM_SKIP_APP_DEPS === "1" || isCfPages;

// patch-package exits 0 on a failed patch unless --error-on-fail is passed (it only
// forces it on CI). --error-on-warn turns a version-mismatch warning into a failure,
// so a dependency bump cannot silently carry a patch made for a different version.
try {
    execSync("npx patch-package --error-on-fail --error-on-warn", { stdio: "inherit" });
} catch (e) {
    // The Cloudflare Pages build is the static component preview, not the shipped
    // Electron app, so an unpatched vendored package there is tolerable.
    if (isCfPages) {
        console.warn("postinstall: patch-package failed (non-fatal for Cloudflare Pages preview build)");
    } else {
        console.error(
            "postinstall: patch-package reported a failed patch or a version mismatch; stopping so the app " +
                "is not built with missing or stale dependency patches.\n" +
                "Check that each file in patches/ matches the exact version pinned in package.json " +
                "and installed in node_modules; regenerate the patch if the dependency was bumped."
        );
        process.exit(1);
    }
}

if (skip) {
    console.log("postinstall: skipping electron-builder install-app-deps");
    process.exit(0);
}

execSync("electron-builder install-app-deps", { stdio: "inherit" });
