#!/bin/bash
set -e

APP_NAME="ClawDesk"
VERSION="${1:-1.0.0}"
VERSION="${VERSION//\//.}"  # 自动将 / 替换为 .
OUTPUT_DIR="dist"

rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

echo "========================================="
echo "  Building $APP_NAME v$VERSION"
echo "========================================="


# =============================================
# macOS builds → DMG
# =============================================
build_macos() {
    local arch=$1
    echo ""
    echo ">>> macOS $arch"

    wails build -platform "darwin/$arch" -o "${APP_NAME}" -clean

    local app_path="build/bin/${APP_NAME}.app"
    if [ ! -d "$app_path" ]; then
        echo "ERROR: $app_path not found"
        return 1
    fi

    local dmg_name="${APP_NAME}-${VERSION}-macos-${arch}.dmg"
    local dmg_path="${OUTPUT_DIR}/${dmg_name}"
    local tmp_dir=$(mktemp -d)

    echo "  Creating DMG: $dmg_name"
    cp -R "$app_path" "$tmp_dir/"
    ln -s /Applications "$tmp_dir/Applications"

    hdiutil create -volname "$APP_NAME" \
        -srcfolder "$tmp_dir" \
        -ov -format UDZO \
        "$dmg_path"

    rm -rf "$tmp_dir"
    echo "  ✅ $dmg_path ($(du -sh "$dmg_path" | cut -f1))"
}

# =============================================
# Windows builds → ZIP (含 exe)
# =============================================
build_windows() {
    local arch=$1
    echo ""
    echo ">>> Windows $arch"

    if [[ "$(uname)" == "Linux" ]]; then
        echo "  ⚠️  跳过：Wails 不支持从 Linux 交叉编译到 Windows，请在 Windows 或 macOS 上构建"
        return 0
    fi

    wails build -platform "windows/$arch" -o "${APP_NAME}.exe" -clean

    local exe_path="build/bin/${APP_NAME}.exe"
    if [ ! -f "$exe_path" ]; then
        echo "ERROR: $exe_path not found"
        return 1
    fi

    # 生成 NSIS 安装脚本
    local nsis_script="/tmp/${APP_NAME}-${arch}.nsi"
    local setup_name="${APP_NAME}-${VERSION}-windows-${arch}-setup.exe"
    local abs_exe_path="$(cd "$(dirname "$exe_path")" && pwd)/$(basename "$exe_path")"
    local abs_output_dir="$(cd "$OUTPUT_DIR" && pwd)"
    local abs_icon=""
    if [ -f "build/windows/icon.ico" ]; then
        abs_icon="$(cd build/windows && pwd)/icon.ico"
    fi

    cat > "$nsis_script" << NSIS
!include "MUI2.nsh"

Name "${APP_NAME}"
OutFile "${abs_output_dir}/${setup_name}"
InstallDir "\$PROGRAMFILES\\${APP_NAME}"
RequestExecutionLevel admin

!define MUI_ICON "${abs_icon:-}"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

Section "Install"
    SetOutPath \$INSTDIR
    File "${abs_exe_path}"

    ; 创建桌面快捷方式
    CreateShortCut "\$DESKTOP\\${APP_NAME}.lnk" "\$INSTDIR\\${APP_NAME}.exe"

    ; 创建开始菜单
    CreateDirectory "\$SMPROGRAMS\\${APP_NAME}"
    CreateShortCut "\$SMPROGRAMS\\${APP_NAME}\\${APP_NAME}.lnk" "\$INSTDIR\\${APP_NAME}.exe"
    CreateShortCut "\$SMPROGRAMS\\${APP_NAME}\\Uninstall.lnk" "\$INSTDIR\\uninstall.exe"

    ; 写入卸载信息
    WriteUninstaller "\$INSTDIR\\uninstall.exe"
    WriteRegStr HKLM "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\${APP_NAME}" "DisplayName" "${APP_NAME}"
    WriteRegStr HKLM "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\${APP_NAME}" "UninstallString" "\$INSTDIR\\uninstall.exe"
    WriteRegStr HKLM "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\${APP_NAME}" "DisplayVersion" "${VERSION}"
SectionEnd

Section "Uninstall"
    Delete "\$INSTDIR\\${APP_NAME}.exe"
    Delete "\$INSTDIR\\uninstall.exe"
    Delete "\$DESKTOP\\${APP_NAME}.lnk"
    RMDir /r "\$SMPROGRAMS\\${APP_NAME}"
    RMDir "\$INSTDIR"
    DeleteRegKey HKLM "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\${APP_NAME}"
SectionEnd
NSIS

    echo "  Creating Setup: $setup_name"

    if command -v makensis &>/dev/null; then
        makensis -V2 "$nsis_script"
    elif command -v docker &>/dev/null; then
        docker run --rm --platform linux/amd64 \
            -v "$(pwd):/work" \
            -v "/tmp:/tmp" \
            -v "$abs_output_dir:/out" \
            --entrypoint makensis \
            binfalse/nsis \
            -V2 "/tmp/${APP_NAME}-${arch}.nsi"
    else
        echo "  ⚠️  未找到 makensis，输出原始 exe（安装 NSIS: brew install nsis）"
        cp "$exe_path" "${OUTPUT_DIR}/${APP_NAME}-${VERSION}-windows-${arch}.exe"
    fi

    rm -f "$nsis_script"

    if [ -f "${OUTPUT_DIR}/${setup_name}" ]; then
        echo "  ✅ ${OUTPUT_DIR}/${setup_name} ($(du -sh "${OUTPUT_DIR}/${setup_name}" | cut -f1))"
    fi
}

# =============================================
# Linux builds → tar.gz（仅 Linux 上可用）
# =============================================
build_linux() {
    local arch=$1
    echo ""
    echo ">>> Linux $arch"

    if [[ "$(uname)" == "Linux" ]]; then
        wails build -platform "linux/$arch" -o "${APP_NAME}" -clean
    else
        if ! command -v docker &>/dev/null; then
            echo "  ⚠️  跳过：需要 Docker（brew install --cask docker）"
            return 0
        fi

        # 构建 Docker 镜像（首次约 3-5 分钟，后续秒完）
        local image_name="clawdesk-linux-builder"
        if ! docker image inspect "$image_name" &>/dev/null; then
            echo "  Building Docker image (first time only)..."
            docker build --platform linux/amd64 -t "$image_name" -f Dockerfile.linux . || return 1
        fi

        echo "  Building in Docker..."
        docker run --rm --platform linux/amd64 \
            -v "$(pwd):/app" \
            -v clawdesk-go-cache:/go \
            -v clawdesk-npm-cache:/root/.npm \
            "$image_name" \
            wails build -platform "linux/amd64" -o "${APP_NAME}" -clean

        # arm64 需要原生构建，amd64 Docker 里无法交叉编译 arm64 GUI 应用
        if [ "$arch" = "arm64" ]; then
            echo "  ⚠️  Linux arm64 需要在 ARM Linux 上构建，已构建 amd64 版本"
        fi
    fi

    local bin_path="build/bin/${APP_NAME}"
    if [ ! -f "$bin_path" ]; then
        echo "  ⚠️  Linux $arch 构建失败，跳过"
        return 0
    fi

    local deb_arch="$arch"
    if [ "$arch" = "amd64" ]; then deb_arch="amd64"; fi
    if [ "$arch" = "arm64" ]; then deb_arch="arm64"; fi

    local deb_name="${APP_NAME}-${VERSION}-linux-${deb_arch}.deb"
    local deb_dir="/tmp/deb-build-$$"
    local app_lower=$(echo "$APP_NAME" | tr '[:upper:]' '[:lower:]')

    echo "  Creating DEB: $deb_name"

    rm -rf "$deb_dir"
    mkdir -p "$deb_dir/DEBIAN"
    mkdir -p "$deb_dir/usr/bin"
    mkdir -p "$deb_dir/usr/share/applications"
    mkdir -p "$deb_dir/usr/share/icons/hicolor/256x256/apps"

    # 控制文件
    cat > "$deb_dir/DEBIAN/control" << CTRL
Package: ${app_lower}
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: ${deb_arch}
Depends: libgtk-3-0, libwebkit2gtk-4.0-37
Maintainer: itmisx <itmisx2025@gmail.com>
Description: ClawDesk - AI Desktop Application
 An all-in-one AI desktop app with multi-agent collaboration,
 skill management, scheduled tasks, and more.
CTRL

    # 复制二进制
    cp "$bin_path" "$deb_dir/usr/bin/${app_lower}"
    chmod 755 "$deb_dir/usr/bin/${app_lower}"

    # 桌面快捷方式
    cat > "$deb_dir/usr/share/applications/${app_lower}.desktop" << DESKTOP
[Desktop Entry]
Name=ClawDesk
Comment=AI Desktop Application
Exec=${app_lower}
Icon=${app_lower}
Type=Application
Categories=Utility;Development;
DESKTOP

    # 图标（如果存在）
    if [ -f "build/appicon.png" ]; then
        cp "build/appicon.png" "$deb_dir/usr/share/icons/hicolor/256x256/apps/${app_lower}.png"
    fi

    # 构建 deb
    if command -v dpkg-deb &>/dev/null; then
        dpkg-deb --build "$deb_dir" "${OUTPUT_DIR}/${deb_name}"
    elif command -v docker &>/dev/null; then
        # macOS 没有 dpkg-deb，用 docker
        docker run --rm -v "$deb_dir:/deb" -v "$(pwd)/${OUTPUT_DIR}:/out" ubuntu:22.04 \
            dpkg-deb --build /deb "/out/${deb_name}"
    else
        # 手动构建（兼容无 dpkg-deb 的环境）
        cd "$deb_dir"
        tar czf ../data.tar.gz --owner=0 --group=0 -C "$deb_dir" usr
        tar czf ../control.tar.gz --owner=0 --group=0 -C "$deb_dir/DEBIAN" .
        echo "2.0" > ../debian-binary
        cd ..
        ar r "$(pwd)/${OUTPUT_DIR}/${deb_name}" debian-binary control.tar.gz data.tar.gz 2>/dev/null || true
        cd "$(pwd)"
    fi

    rm -rf "$deb_dir"

    if [ -f "${OUTPUT_DIR}/${deb_name}" ]; then
        echo "  ✅ ${OUTPUT_DIR}/${deb_name} ($(du -sh "${OUTPUT_DIR}/${deb_name}" | cut -f1))"
    fi
}

# =============================================
# 入口
# =============================================
TARGETS="${2:-all}"

case "$TARGETS" in
    all)
        build_macos arm64
        build_macos amd64
        build_windows arm64
        build_windows amd64
        build_linux arm64
        build_linux amd64
        ;;
    macos-arm64)    build_macos arm64 ;;
    macos-amd64)    build_macos amd64 ;;
    windows-arm64)  build_windows arm64 ;;
    windows-amd64)  build_windows amd64 ;;
    linux-arm64)    build_linux arm64 ;;
    linux-amd64)    build_linux amd64 ;;
    *)
        echo "Usage: $0 [version] [target]"
        echo ""
        echo "Targets:"
        echo "  macos-arm64      macOS Apple Silicon → DMG"
        echo "  macos-amd64      macOS Intel → DMG"
        echo "  windows-arm64    Windows ARM64 → ZIP"
        echo "  windows-amd64    Windows x64 → ZIP"
        echo "  linux-arm64      Linux ARM64 → tar.gz"
        echo "  linux-amd64      Linux x64 → tar.gz"
        echo ""
        echo "Examples:"
        echo "  $0 1.0.0 macos-arm64"
        echo "  $0 1.0.0 windows-amd64"
        exit 1
        ;;
esac

echo ""
echo "========================================="
echo "  Build complete!"
echo "========================================="
ls -lh "$OUTPUT_DIR/"
