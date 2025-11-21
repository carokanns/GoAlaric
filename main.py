import json
import os
import re
from typing import List, Tuple, Set


def extract_functions(source: str) -> List[Tuple[str, str]]:
    """
    Extract Go function definitions from a source string.

    Returns a list of tuples containing the function name and the full
    function body (including the signature and braces). Both free functions
    and methods with receivers are detected. The extraction attempts to
    correctly match opening and closing braces, ignoring braces inside
    string literals and character literals.
    """
    # Regular expression to find function signatures. This matches both
    # standalone functions (e.g. "func Foo(") and methods with receivers
    # (e.g. "func (r Receiver) Foo("). We capture the function name in
    # group 1.
    func_pattern = re.compile(
        r"func\s+(?:\(\s*[\w\*\s]+\)\s*)?([A-Za-z_][\w]*)\s*\(",
        re.MULTILINE,
    )

    results: List[Tuple[str, str]] = []

    for match in func_pattern.finditer(source):
        func_name = match.group(1)
        start_pos = match.start()
        # Find the position of the opening brace for the function body
        brace_pos = source.find('{', match.end())
        if brace_pos == -1:
            # No opening brace found; skip this function
            continue
        # Initialize tracking variables for brace matching
        brace_count = 0
        i = brace_pos
        in_string: str | None = None
        escaped = False

        # Walk through the source code character by character starting from
        # the opening brace. Increase brace_count on '{' and decrease on '}'.
        # Stop when brace_count returns to zero (the matching closing brace).
        while i < len(source):
            char = source[i]
            if escaped:
                # Skip the character if it was escaped (e.g. \")
                escaped = False
            else:
                if char == '\\':
                    # Backslash indicates an escape in strings
                    escaped = True
                elif in_string:
                    # If currently inside a string literal, look for its end
                    if char == in_string:
                        in_string = None
                else:
                    # Not inside a string literal; detect start of strings or braces
                    if char in ('"', "'"):
                        # Enter a string literal
                        in_string = char
                    elif char == '{':
                        brace_count += 1
                    elif char == '}':
                        brace_count -= 1
                        if brace_count == 0:
                            # Matching closing brace found
                            end_pos = i + 1
                            results.append((func_name, source[start_pos:end_pos]))
                            break
            i += 1
    return results


def clean_comments(code: str) -> str:
    """
    Remove line comments (// ...) and block comments (/* ... */) from Go code.
    This helps to avoid detecting function calls that appear inside comments.
    """
    # Remove line comments
    code_no_line_comments = re.sub(r'//.*', '', code)
    # Remove block comments
    code_no_comments = re.sub(r'/\*.*?\*/', '', code_no_line_comments, flags=re.DOTALL)
    return code_no_comments


def find_calls(function_body: str, current_function: str) -> List[str]:
    """
    Find all function or method calls within a Go function body.

    This function returns a sorted list of unique call identifiers. It
    distinguishes between unqualified calls (e.g. foo()) and qualified calls
    (e.g. pkg.Func()). It ignores keywords and avoids counting the function
    definition itself as a call.
    """
    # Remove comments to avoid picking up calls from commented-out code
    text = clean_comments(function_body)

    calls: Set[str] = set()

    # Pattern to match qualified calls such as pkg.Func(
    pattern_pkg = re.compile(r'\b([A-Za-z_][\w]*)\s*\.\s*([A-Za-z_][\w]*)\s*\(')
    for match in pattern_pkg.finditer(text):
        pkg_name, func_name = match.group(1), match.group(2)
        calls.add(f"{pkg_name}.{func_name}")

    # Pattern to match unqualified calls. Use negative lookbehinds to
    # ensure the call is not preceded by a dot (to avoid counting
    # methods or package-qualified calls) and not immediately preceded by
    # the keyword "func " (to avoid matching the function definition line).
    pattern_unq = re.compile(
        r'(?<!\.)'      # not preceded by a dot
        r'(?<!func\s)'  # not preceded by 'func ' (function definition)
        r'\b([A-Za-z_][\w]*)\s*\(',
        re.MULTILINE,
    )

    # Keywords and built-ins that should not be counted as function calls
    ignore_keywords = {
        'if', 'for', 'switch', 'select', 'return', 'go', 'defer',
        'else', 'case', 'range', 'type', 'struct', 'interface', 'func', 'break', 'continue',
    }

    for match in pattern_unq.finditer(text):
        func_name = match.group(1)
        # Skip keywords and package aliases used in import alias declarations
        if func_name in ignore_keywords:
            continue
        calls.add(func_name)

    # Remove calls to the current function itself (avoid counting the function
    # definition as a call)
    calls.discard(current_function)

    return sorted(calls)


def analyze_directory(root: str) -> List[Tuple[str, str, str, str]]:
    """
    Traverse the given root directory, analyze Go files, and return a list of
    rows containing:

      (relative_directory, filename, function_name, comma_separated_calls)

    The relative_directory is reported as 'Root' when it refers to the root
    itself. Calls within functions are determined by scanning the function
    bodies for qualified and unqualified call patterns.
    """
    rows: List[Tuple[str, str, str, str]] = []
    for dirpath, dirnames, filenames in os.walk(root):
        for filename in filenames:
            if not filename.lower().endswith('.go'):
                continue
            file_path = os.path.join(dirpath, filename)
            try:
                with open(file_path, 'r', encoding='utf-8') as f:
                    source = f.read()
            except (OSError, UnicodeDecodeError):
                # Skip files that cannot be read or decoded
                continue
            functions = extract_functions(source)
            for func_name, func_body in functions:
                calls = find_calls(func_body, func_name)
                relative_dir = os.path.relpath(dirpath, root)
                # Normalize directory name: use 'Root' when at top level
                if relative_dir == '.' or relative_dir == '':
                    relative_dir_display = 'Root'
                else:
                    relative_dir_display = relative_dir.replace(os.sep, '/')
                rows.append((relative_dir_display, filename, func_name, ', '.join(calls)))
    return rows


def main() -> None:
    """
    Entry point for command-line execution. Accepts an optional directory
    argument (defaulting to the current working directory) and prints either
    a tab-separated table or JSON of function dependencies in Go files.
    """
    import argparse

    parser = argparse.ArgumentParser(
        description=(
            'Analyze Go source files in a directory and list each function\n'
            'along with the functions it calls. Outputs either a tab-separated\n'
            'table (default) or JSON with one entry per function.'
        )
    )
    parser.add_argument(
        'directory', nargs='?', default='.',
        help='Root directory to analyze (default: current directory)',
    )
    parser.add_argument(
        '--format', '-f', choices=('tsv', 'json'), default='tsv',
        help='Output format: "tsv" (default) or "json".',
    )
    args = parser.parse_args()

    rows = analyze_directory(args.directory)
    if args.format == 'json':
        payload = [
            {
                'mapp': row[0],
                'go-fil': row[1],
                'funktionsnamn': row[2],
                'anropar': row[3],
            }
            for row in rows
        ]
        print(json.dumps(payload))
    else:
        # Default to TSV so existing workflows continue to work.
        print('mapp\tgo-fil\tfunktionsnamn\tanropar')
        for row in rows:
            print('\t'.join(row))


if __name__ == '__main__':
    main()
