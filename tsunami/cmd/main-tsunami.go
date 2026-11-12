package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/LannCo/remoteterm/tsunami/build"
	"github.com/LannCo/remoteterm/tsunami/tsunamibase"
	"github.com/spf13/cobra"
)

const (
	EnvTsunamiScaffoldPath   = "TSUNAMI_SCAFFOLDPATH"
	EnvTsunamiSdkReplacePath = "TSUNAMI_SDKREPLACEPATH"
	EnvTsunamiNodePath       = "TSUNAMI_NODEPATH"
	TsunamiSdkVersion        = "v0.12.4"
)

// these are set at build time
var TsunamiVersion = "0.0.0"
var BuildTime = "0"

var rootCmd = &cobra.Command{
	Use:   "tsunami",
	Short: "Tsunami - A VDOM-based UI framework",
	Long:  `Tsunami is a VDOM-based UI framework for building modern applications.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print Tsunami version",
	Long:  `Print Tsunami version`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("v" + tsunamibase.TsunamiVersion)
	},
}

func validateEnvironmentVars(opts *build.BuildOpts) error {
	scaffoldPath := os.Getenv(EnvTsunamiScaffoldPath)
	if scaffoldPath == "" {
		return fmt.Errorf("%s environment variable must be set", EnvTsunamiScaffoldPath)
	}
	absScaffoldPath, err := filepath.Abs(scaffoldPath)
	if err != nil {
		return fmt.Errorf("failed to resolve %s to absolute path: %w", EnvTsunamiScaffoldPath, err)
	}

	sdkReplacePath := os.Getenv(EnvTsunamiSdkReplacePath)
	if sdkReplacePath == "" {
		return fmt.Errorf("%s environment variable must be set", EnvTsunamiSdkReplacePath)
	}
	absSdkReplacePath, err := filepath.Abs(sdkReplacePath)
	if err != nil {
		return fmt.Errorf("failed to resolve %s to absolute path: %w", EnvTsunamiSdkReplacePath, err)
	}

	opts.ScaffoldPath = absScaffoldPath
	opts.SdkReplacePath = absSdkReplacePath

	// NodePath is optional
	if nodePath := os.Getenv(EnvTsunamiNodePath); nodePath != "" {
		opts.NodePath = nodePath
	}

	return nil
}

var buildCmd = &cobra.Command{
	Use:          "build [apppath]",
	Short:        "Build a Tsunami application",
	Long:         `Build a Tsunami application.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")
		keepTemp, _ := cmd.Flags().GetBool("keeptemp")
		output, _ := cmd.Flags().GetString("output")
		opts := build.BuildOpts{
			AppPath:      args[0],
			Verbose:      verbose,
			KeepTemp:     keepTemp,
			OutputFile:   output,
			MoveFileBack: true,
			SdkVersion:   TsunamiSdkVersion,
		}
		if err := validateEnvironmentVars(&opts); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if err := build.TsunamiBuild(opts); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

var runCmd = &cobra.Command{
	Use:          "run [apppath]",
	Short:        "Build and run a Tsunami application",
	Long:         `Build and run a Tsunami application.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")
		open, _ := cmd.Flags().GetBool("open")
		keepTemp, _ := cmd.Flags().GetBool("keeptemp")
		opts := build.BuildOpts{
			AppPath:      args[0],
			Verbose:      verbose,
			Open:         open,
			KeepTemp:     keepTemp,
			MoveFileBack: true,
			SdkVersion:   TsunamiSdkVersion,
		}
		if err := validateEnvironmentVars(&opts); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if err := build.TsunamiRun(opts); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

var packageCmd = &cobra.Command{
	Use:          "package [apppath]",
	Short:        "Package a Tsunami application into a .tsapp file",
	Long:         `Package a Tsunami application into a .tsapp file.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")
		output, _ := cmd.Flags().GetString("output")
		appPath := args[0]

		if output == "" {
			appName := build.GetAppName(appPath)
			output = filepath.Join(appPath, appName+".tsapp")
		}

		appFS := build.NewDirFS(appPath)
		if err := build.MakeAppPackage(appFS, appPath, verbose, output); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		if verbose {
			fmt.Printf("Successfully created package: %s\n", output)
		}
	},
}

var sdkBundleCmd = &cobra.Command{
	Use:          "sdkbundle [dstdir]",
	Short:        "Copy the Tsunami SDK runtime packages into a directory",
	Long:         `Copy go.mod, go.sum and the runtime packages of the Tsunami SDK into a directory that app builds can use as a replace target.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		srcDir, _ := cmd.Flags().GetString("src")
		if err := build.CopySdkBundle(srcDir, args[0]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var goToolchainCmd = &cobra.Command{
	Use:          "gotoolchain",
	Short:        "Stage a trimmed Go toolchain for the packaged app",
	Long:         `Download a Go release (verified against a pinned SHA-256) or trim an installed one, keeping only what compiling an app needs.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		out, _ := cmd.Flags().GetString("out")
		fromDir, _ := cmd.Flags().GetString("from-dir")
		var err error
		if fromDir != "" {
			err = build.TrimToolchainDir(fromDir, out)
		} else {
			version, _ := cmd.Flags().GetString("version")
			goos, _ := cmd.Flags().GetString("os")
			goarch, _ := cmd.Flags().GetString("arch")
			sum, _ := cmd.Flags().GetString("sha256")
			err = build.FetchToolchain(context.Background(), build.FetchToolchainOpts{
				Version: version, GOOS: goos, GOARCH: goarch, DstRoot: out, SHA256: sum,
			})
			if err == nil {
				err = build.VerifyToolchainArch(out, goos, goarch)
			}
		}
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var goModCacheCmd = &cobra.Command{
	Use:          "gomodcache",
	Short:        "Stage the SDK's module dependencies as a proxy directory for the packaged app",
	Long:         `Resolve the SDK's dependencies online once and copy them into a directory laid out as a Go module proxy, so a first build in the packaged app needs no network for them.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		sdkDir, _ := cmd.Flags().GetString("sdk")
		out, _ := cmd.Flags().GetString("out")
		goPath, _ := cmd.Flags().GetString("go")
		proxy, _ := cmd.Flags().GetString("proxy")
		err := build.StageModuleCache(context.Background(), build.ModCacheOpts{GoPath: goPath, SdkDir: sdkDir, OutDir: out, Proxy: proxy})
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)

	goToolchainCmd.Flags().String("out", "", "Directory to create (replaced if it exists)")
	goToolchainCmd.Flags().String("version", build.PinnedToolchainVersion, "Go release to download")
	goToolchainCmd.Flags().String("os", "darwin", "GOOS of the release to download")
	goToolchainCmd.Flags().String("arch", "arm64", "GOARCH of the release to download")
	goToolchainCmd.Flags().String("sha256", "", "Checksum for a version that is not pinned in the source")
	goToolchainCmd.Flags().String("from-dir", "", "Trim this installed Go root instead of downloading")
	_ = goToolchainCmd.MarkFlagRequired("out")
	rootCmd.AddCommand(goToolchainCmd)

	goModCacheCmd.Flags().String("sdk", "", "SDK bundle directory (the output of sdkbundle)")
	goModCacheCmd.Flags().String("out", "", "Directory to create (replaced if it exists)")
	goModCacheCmd.Flags().String("go", "", "Go binary to resolve with")
	goModCacheCmd.Flags().String("proxy", "", "GOPROXY to fetch from (default: the public Go proxy)")
	_ = goModCacheCmd.MarkFlagRequired("sdk")
	_ = goModCacheCmd.MarkFlagRequired("out")
	_ = goModCacheCmd.MarkFlagRequired("go")
	rootCmd.AddCommand(goModCacheCmd)

	buildCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")
	buildCmd.Flags().Bool("keeptemp", false, "Keep temporary build directory")
	buildCmd.Flags().StringP("output", "o", "", "Output file path for the built application")
	rootCmd.AddCommand(buildCmd)

	runCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")
	runCmd.Flags().Bool("open", false, "Open the application in the browser after starting")
	runCmd.Flags().Bool("keeptemp", false, "Keep temporary build directory")
	rootCmd.AddCommand(runCmd)

	packageCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")
	packageCmd.Flags().StringP("output", "o", "", "Output file path for the package (default: [appname].tsapp in apppath)")
	rootCmd.AddCommand(packageCmd)

	sdkBundleCmd.Flags().String("src", ".", "Tsunami SDK source directory (the one containing go.mod)")
	rootCmd.AddCommand(sdkBundleCmd)
}

func main() {
	tsunamibase.TsunamiVersion = TsunamiVersion
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
