<p align="center">
  <a href="https://remoteterm.dev">
	<picture>
		<source media="(prefers-color-scheme: dark)" srcset="./assets/wave-dark.png">
		<source media="(prefers-color-scheme: light)" srcset="./assets/wave-light.png">
		<img alt="RemoteTerm Logo" src="./assets/wave-light.png" width="240">
	</picture>
  </a>
  <br/>
</p>

# RemoteTerm

<div align="center">

[English](README.md) | [한국어](README.ko.md) | [繁體中文](README.zh-TW.md)

</div>

[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fwavetermdev%2Fwaveterm.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fwavetermdev%2Fwaveterm?ref=badge_shield)

> 이 문서는 커뮤니티 한국어 번역본입니다. 최신 원문은 [README.md](README.md)에서 확인하세요.

RemoteTerm은 macOS, Linux, Windows에서 동작하는 오픈소스 터미널입니다. 계정 생성은 필요하지 않습니다.

또한 RemoteTerm은 네트워크 중단이나 재시작 이후에도 유지되는 내구성 있는 SSH 세션을 지원하며, 자동 재연결 기능을 제공합니다. 내장 그래픽 에디터로 원격 파일을 편집하고, 터미널을 벗어나지 않고도 파일을 인라인으로 미리볼 수 있습니다.

![RemoteTerm Screenshot](./assets/wave-screenshot.webp)

## 주요 기능

- 내구성 있는 SSH 세션 - 연결 끊김, 네트워크 변경, RemoteTerm 재시작 상황에서도 자동 재연결로 세션 유지
- 터미널 블록, 에디터, 웹 브라우저, 미리보기를 유연하게 배치할 수 있는 드래그 앤 드롭 인터페이스
- 구문 강조와 최신 편집 기능을 제공하는 원격 파일 편집용 내장 에디터
- 원격 파일용 풍부한 미리보기 시스템 (Markdown, 이미지, 동영상, PDF, CSV, 디렉터리)
- 블록 단위 빠른 전체 화면 토글 - 터미널/에디터/미리보기를 크게 보고 즉시 멀티 블록 보기로 복귀
- 개별 명령을 분리하고 모니터링할 수 있는 Command Blocks
- 한 번의 클릭으로 원격 연결 및 전체 터미널/파일 시스템 접근
- 네이티브 시스템 백엔드를 사용하는 안전한 시크릿 저장 - API 키와 자격 증명을 로컬에 저장하고 SSH 세션 간 공유
- 탭 테마, 터미널 스타일, 배경 이미지 등 폭넓은 커스터마이징
- CLI에서 워크스페이스를 제어하고 세션 간 데이터를 공유하는 강력한 `wsh` 명령 시스템
- `wsh file`을 통한 연결형 파일 관리 - 로컬과 원격 SSH 호스트 간 파일 복사/동기화

## 설치

RemoteTerm은 macOS, Linux, Windows에서 동작합니다.

플랫폼별 설치 방법은 [여기](https://docs.waveterm.dev/gettingstarted)에서 확인할 수 있습니다.

직접 다운로드하여 설치하려면 [www.waveterm.dev/download](https://www.waveterm.dev/download)을 이용하세요.

### 최소 요구 사항

RemoteTerm은 다음 플랫폼에서 실행됩니다.

- macOS 11 이상 (arm64, x64)
- Windows 10 1809 이상 (x64)
- glibc-2.28 이상 기반 Linux (Debian 10, RHEL 8, Ubuntu 20.04 등) (arm64, x64)

WSH 헬퍼는 다음 플랫폼에서 실행됩니다.

- macOS 11 이상 (arm64, x64)
- Windows 10 이상 (x64)
- Linux Kernel 2.6.32 이상 (x64), Linux Kernel 3.1 이상 (arm64)

## 로드맵

RemoteTerm은 계속 발전하고 있습니다. 로드맵은 릴리스 목표에 맞춰 지속적으로 업데이트됩니다. [여기](./ROADMAP.md)에서 확인하세요.

향후 릴리스 방향에 의견을 주고 싶다면 [Discord](https://discord.gg/XfvZ334gwU)에 참여하거나 [Feature Request](https://github.com/LannCo/remoteterm/issues/new/choose)를 등록해 주세요.

## 링크

- 홈페이지 &mdash; https://remoteterm.dev
- 다운로드 페이지 &mdash; https://www.waveterm.dev/download
- 문서 &mdash; https://docs.waveterm.dev
- X &mdash; https://x.com/wavetermdev
- Discord 커뮤니티 &mdash; https://discord.gg/XfvZ334gwU

## 소스에서 빌드

[Building RemoteTerm](BUILD.md)을 참고하세요.

## 기여하기

RemoteTerm은 GitHub Issues를 이슈 추적에 사용합니다.

[기여 가이드](CONTRIBUTING.md)에서 더 많은 정보를 확인할 수 있습니다.

- [기여 방법](CONTRIBUTING.md#contributing-to-remoteterm)
- [기여 가이드라인](CONTRIBUTING.md#high-level-expectations)

## 라이선스

RemoteTerm은 Apache-2.0 라이선스를 따릅니다. 의존성 정보는 [여기](./ACKNOWLEDGEMENTS.md)에서 확인할 수 있습니다.
