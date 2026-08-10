#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tools_dir="$repo_root/.tools"
fathom_commit="c9c6fef0dddc05d2e242c183acf5833149ab676d"
fathom_src="$tools_dir/fathom-src"
fathom_bin="$tools_dir/fathom/bin/fathom"
syzygy_dir="$tools_dir/syzygy/3-4-5"
python_dir="$tools_dir/python"
base_url="http://tablebase.sesse.net/syzygy/3-4-5"

mkdir -p "$tools_dir" "$syzygy_dir" "$python_dir" "$(dirname "$fathom_bin")"

if [[ ! -d "$fathom_src/.git" ]]; then
  git clone https://github.com/jdart1/Fathom.git "$fathom_src"
fi
git -C "$fathom_src" fetch --quiet origin "$fathom_commit"
git -C "$fathom_src" checkout --quiet "$fathom_commit"
make -C "$fathom_src/src/apps"
cp "$fathom_src/src/apps/fathom.linux" "$fathom_bin"

if ! PYTHONPATH="$python_dir" python3 -c 'import chess; assert chess.__version__ == "1.11.2"' 2>/dev/null; then
  python3 -m pip install --disable-pip-version-check --target "$python_dir" chess==1.11.2
fi

tables=(
  "99bf9b05295781611cdd7c5c3d51bf85 KBvK.rtbw"
  "88d5f823e67448b279bb045977a80a39 KBvK.rtbz"
  "b6781a75ffe2ab41507f91151869a418 KNvK.rtbw"
  "42893523156bbc5d8c3c7207a7710ad7 KNvK.rtbz"
  "f06221548404795b6b33469e247b4560 KQvK.rtbw"
  "ac866466e16eb19a4f8c796f8e1abd2b KQvK.rtbz"
  "89a27823bfa03d0b0f25728c4f0fb571 KRvK.rtbw"
  "9cb0795fa43904a3e91bf749971964b8 KRvK.rtbz"
  "2ebcb69aa6056b3aa6850ee361de6068 KBvKB.rtbw"
  "9ce5a78c4b4658d9ff491c36120f6132 KBvKB.rtbz"
  "267d1315e295cebd435ecec0098a2a44 KNvKN.rtbw"
  "fec4e44f3033ac6b7ded59a3b74e29a5 KNvKN.rtbz"
  "b7118e15bcf741db15f2bd09a0c638c9 KQvKB.rtbw"
  "4c4e0dbfc725ed89e99196c2a38b8260 KQvKB.rtbz"
  "87ec78000e4413e1afeda4838402afbd KQvKN.rtbw"
  "d694bcf4e8c771ed3ab7140e9072ad4d KQvKN.rtbz"
  "c8add566d88bfbc4328db6de3be5cc4b KRvKB.rtbw"
  "ced507ec717b6031f42ef53cd1df860b KRvKB.rtbz"
  "5f2c91dd8fa2e6fdf2664560d4d02bc2 KRvKN.rtbw"
  "f8b6e95250e5f6657e0333a43dc43251 KRvKN.rtbz"
  "e9390be76079250caba06f353f758536 KPvKP.rtbw"
  "17cec6d51197c92b97b3ad9bc5559ee0 KPvKP.rtbz"
  "46f6ef491bd26696d7b20281e7c5b721 KPvK.rtbw"
  "54460894c15f087cfd16670bf1513755 KPvK.rtbz"
)
for entry in "${tables[@]}"; do
  read -r expected table <<<"$entry"
  target="$syzygy_dir/$table"
  if [[ -s "$target" ]] && echo "$expected  $target" | md5sum --quiet --check -; then
    continue
  fi
  partial="$target.partial"
  curl -fsSL --retry 3 -o "$partial" "$base_url/$table"
  echo "$expected  $partial" | md5sum --check -
  mv "$partial" "$target"
done

echo "Fathom: $fathom_bin"
echo "Syzygy: $syzygy_dir"
echo "Python: $python_dir"
