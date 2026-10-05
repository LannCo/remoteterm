const { Arch } = require("electron-builder");
const pkg = require("./package.json");
const stagedResources = require("./build/staged-resources.cjs");

// Fails here, before electron-builder spends minutes packaging an app without the SDK.
// The config unit test loads this file on a tree that has not staged anything and sets the
// variable; a real packaging run never does.
if (!process.env.REMOTETERM_CONFIG_TEST) {
    stagedResources.failIfAny(stagedResources.checkSdk(__dirname));
}

const windowsShouldSign = !!process.env.SM_CODE_SIGNING_CERT_SHA1_HASH;

/**
 * @type {import('electron-builder').Configuration}
 * @see https://www.electron.build/configuration/configuration
 */
const config = {
    appId: pkg.build.appId,
    productName: pkg.productName,
    executableName: pkg.productName,
    artifactName: "${productName}-${platform}-${arch}-${version}.${ext}",
    generateUpdatesFilesForAllChannels: true,
    npmRebuild: false,
    nodeGypRebuild: false,
    electronCompile: false,
    files: [
        {
            from: "./dist",
            to: "./dist",
            filter: [
                "**/*",
                "!bin/*",
                "bin/remotetermsrv.${arch}*",
                "bin/wsh*",
                "!tsunamiscaffold/**/*",
                "!tsunamiscaffold-*/**/*",
                "!tsunamisdk/**/*",
                "!gotoolchain-*/**/*",
                "!gomodcache/**/*",
            ],
        },
        {
            from: ".",
            to: ".",
            filter: ["package.json"],
        },
        "!node_modules", // We don't need electron-builder to package in Node modules as Vite has already bundled any code that our program is using.
    ],
    // The scaffold's node_modules holds native Tailwind binaries, so it is per platform:
    // Linux and Windows ship the one built on the packaging host; each macOS architecture
    // ships its own (see build/stage-scaffold-natives.sh), plus the Go toolchain and module
    // cache that let the app compile builder apps with no Go or network on the machine.
    extraResources: [
        {
            from: "dist/tsunamisdk",
            to: "tsunamisdk",
        },
    ],
    directories: {
        output: "make",
    },
    asarUnpack: [
        "dist/bin/**/*", // remotetermsrv and wsh binaries
        "dist/schema/**/*", // schema files for Monaco editor
    ],
    mac: {
        target: [
            {
                target: "zip",
                arch: ["arm64", "x64"],
            },
            {
                target: "dmg",
                arch: ["arm64", "x64"],
            },
        ],
        category: "public.app-category.developer-tools",
        minimumSystemVersion: "12.0.0", // Electron 38 and later need macOS 12
        extraResources: [
            {
                from: "dist/tsunamiscaffold-${arch}",
                to: "tsunamiscaffold",
            },
            {
                from: "dist/gotoolchain-${arch}",
                to: "gotoolchain",
            },
            {
                from: "dist/gomodcache",
                to: "gomodcache",
            },
        ],
        entitlements: "build/entitlements.mac.plist",
        entitlementsInherit: "build/entitlements.mac.plist",
        extendInfo: {
            NSContactsUsageDescription: "A CLI application running in RemoteTerm wants to use your contacts.",
            NSRemindersUsageDescription: "A CLI application running in RemoteTerm wants to use your reminders.",
            NSLocationWhenInUseUsageDescription:
                "A CLI application running in RemoteTerm wants to use your location information while active.",
            NSLocationAlwaysUsageDescription:
                "A CLI application running in RemoteTerm wants to use your location information, even in the background.",
            NSCameraUsageDescription: "A CLI application running in RemoteTerm wants to use the camera.",
            NSMicrophoneUsageDescription: "A CLI application running in RemoteTerm wants to use your microphone.",
            NSCalendarsUsageDescription: "A CLI application running in RemoteTerm wants to use Calendar data.",
            NSLocationUsageDescription:
                "A CLI application running in RemoteTerm wants to use your location information.",
            NSAppleEventsUsageDescription: "A CLI application running in RemoteTerm wants to use AppleScript.",
        },
    },
    linux: {
        artifactName: "${name}-${platform}-${arch}-${version}.${ext}",
        category: "TerminalEmulator",
        executableName: pkg.name,
        target: ["zip", "deb", "rpm", "snap", "AppImage", "pacman"],
        synopsis: pkg.description,
        description: null,
        desktop: {
            entry: {
                Name: pkg.productName,
                Comment: pkg.description,
                Keywords: "developer;terminal;emulator;",
                Categories: "Development;Utility;",
            },
        },
        executableArgs: ["--enable-features", "UseOzonePlatform", "--ozone-platform-hint", "auto"], // Hint Electron to use Ozone abstraction layer for native Wayland support
        extraResources: [
            {
                from: "dist/tsunamiscaffold",
                to: "tsunamiscaffold",
            },
        ],
    },
    deb: {
        afterInstall: "build/deb-postinstall.tpl",
    },
    win: {
        extraResources: [
            {
                from: "dist/tsunamiscaffold",
                to: "tsunamiscaffold",
            },
        ],
        target: ["nsis", "msi", "zip"],
        signtoolOptions: windowsShouldSign && {
            signingHashAlgorithms: ["sha256"],
            publisherName: "Command Line Inc",
            certificateSubjectName: "Command Line Inc",
            certificateSha1: process.env.SM_CODE_SIGNING_CERT_SHA1_HASH,
        },
    },
    appImage: {
        license: "LICENSE",
    },
    snap: {
        base: "core22",
        confinement: "classic",
        allowNativeWayland: true,
        artifactName: "${name}_${version}_${arch}.${ext}",
    },
    rpm: {
        // this should remove /usr/lib/.build-id/ links which can conflict with other electron apps like slack
        fpm: ["--rpm-rpmbuild-define", "_build_id_links none"],
    },
    beforePack: (context) => {
        if (context.electronPlatformName === "darwin") {
            stagedResources.failIfAny(stagedResources.checkMacArch(__dirname, Arch[context.arch]));
        }
    },
};

module.exports = config;
