import 'package:material_ui/material_ui.dart';
import 'package:nectar_apps_ui/widgets/highlights/syntax_rule.dart';

final tomlSyntaxRules = <SyntaxRule>[
  // Comments
  SyntaxRule(
    RegExp(r'(?<!\S)#[^\n]*'),
    const TextStyle(color: Color(0xFF7F848E), fontStyle: FontStyle.italic),
  ),

  // Table and array table headers
  SyntaxRule(
    RegExp(r'^\s*\[\[?[^\]\n]+\]\]?'),
    const TextStyle(color: Color(0xFFC678DD), fontWeight: FontWeight.bold),
  ),

  // Basic and literal strings, including multiline
  SyntaxRule(
    RegExp(
      r'''(?:'''
      r'"""[\s\S]*?"""'
      r"""|'''[\s\S]*?'''|"(?:[^"\\]|\\.)*"|'[^']*')""",
    ),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // Keys
  SyntaxRule(
    RegExp(r'^[ \t]*[a-zA-Z0-9_-]+(?=\s*=)'),
    const TextStyle(color: Color(0xFF56B6C2)),
  ),

  // Booleans
  SyntaxRule(
    RegExp(r'\b(true|false)\b'),
    const TextStyle(color: Color(0xFFC678DD)),
  ),

  // Date and time values
  SyntaxRule(
    RegExp(
      r'\b\d{4}-\d{2}-\d{2}'
      r'(?:[Tt ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:[Zz]|[+-]\d{2}:\d{2})?)?\b'
      r'|\b\d{2}:\d{2}:\d{2}(?:\.\d+)?\b',
    ),
    const TextStyle(color: Color(0xFFE5C07B)),
  ),

  // Numbers, including hexadecimal, octal and binary
  SyntaxRule(
    RegExp(
      r'[+-]?(?:0x[0-9a-fA-F_]+|0o[0-7_]+|0b[01_]+|'
      r'\d[\d_]*(?:\.[\d_]*)?(?:[eE][+-]?\d[\d_]*)?)',
    ),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // Assignment
  SyntaxRule(RegExp(r'='), const TextStyle(color: Color(0xFFE06C75))),

  // Punctuation
  SyntaxRule(RegExp(r'[{}\[\],.]'), const TextStyle(color: Color(0xFFABB2BF))),
];
