import 'package:flutter/material.dart';
import 'package:nectar_apps_ui/widgets/highlights/syntax_rule.dart';

final shellSyntaxRules = <SyntaxRule>[
  // Comments
  SyntaxRule(
    RegExp(r'(^|(?<=\s))#[^\n]*'),
    const TextStyle(color: Color(0xFF7F848E), fontStyle: FontStyle.italic),
  ),

  // Single and double quoted strings
  SyntaxRule(
    RegExp(r''''(?:[^'\\]|\\.)*'|"(?:[^"\\]|\\.)*"'''),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // ANSI-C quoted strings: $'...'
  SyntaxRule(
    RegExp(r'''\$'(?:[^'\\]|\\.)*'|\\["'\\$`]'''),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // Command substitutions
  SyntaxRule(
    RegExp(r'\$\([^)]+\)|`[^`]*`'),
    const TextStyle(color: Color(0xFFE5C07B)),
  ),

  // Environment variables
  SyntaxRule(
    RegExp(r'\b[A-Z_][A-Z0-9_]*(?==)|\$\{?[A-Z_][A-Z0-9_]*\}?'),
    const TextStyle(color: Color(0xFF56B6C2)),
  ),

  // Shell variables
  SyntaxRule(
    RegExp(r'\$\{[^}]+\}|\$[a-zA-Z_][a-zA-Z0-9_]*'),
    const TextStyle(color: Color(0xFF61AFEF)),
  ),

  // Special shell variables
  SyntaxRule(
    RegExp(r'\$[@#?$!0*-]|\\[ntr]'),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // Shell keywords
  SyntaxRule(
    RegExp(
      r'\b(if|then|elif|else|fi|for|while|until|do|done|'
      r'case|esac|in|function|select|time|coproc|'
      r'break|continue|return|exit|local|export|readonly|'
      r'declare|typeset|unset|shift|source|eval|chmod|exec|chown|'
      r'trap|set|sh|shopt|alias|unalias)\b',
    ),
    const TextStyle(color: Color(0xFFC678DD), fontWeight: FontWeight.bold),
  ),

  // Common shell builtins
  SyntaxRule(
    RegExp(
      r'\b(cd|echo|printf|read|pwd|type|test|'
      r'wait|jobs|fg|bg|kill|getopts|umask|'
      r'pushd|popd|dirs|help|history)\b',
    ),
    const TextStyle(color: Color(0xFF56B6C2)),
  ),

  // Flags and options
  SyntaxRule(
    RegExp(r'(?<!\S)--?[a-zA-Z][a-zA-Z0-9_-]*'),
    const TextStyle(color: Color(0xFF61AFEF)),
  ),

  // Numbers
  SyntaxRule(
    RegExp(r'\b\d+(\.\d+)?\b'),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // Redirection and pipes
  SyntaxRule(
    RegExp(r'<<<|<<|>>|>&|<&|[|&;<>]'),
    const TextStyle(color: Color(0xFFE06C75)),
  ),

  // Operators
  SyntaxRule(
    RegExp(r'==|!=|<=|>=|&&|\|\||\+=|-=|='),
    const TextStyle(color: Color(0xFFE06C75)),
  ),
];
