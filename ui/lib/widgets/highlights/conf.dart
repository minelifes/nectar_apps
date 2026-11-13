import 'package:material_ui/material_ui.dart';
import 'package:nectar_apps_ui/widgets/highlights/syntax_rule.dart';

final confSyntaxRules = <SyntaxRule>[
  // Comments
  SyntaxRule(
    RegExp(r'^\s*[#;].*|(?<=\s)[#;].*'),
    const TextStyle(color: Color(0xFF7F848E), fontStyle: FontStyle.italic),
  ),

  // Section headers
  SyntaxRule(
    RegExp(r'^\s*\[[^\]\n]+\]'),
    const TextStyle(color: Color(0xFFC678DD), fontWeight: FontWeight.bold),
  ),

  // Keys
  SyntaxRule(
    RegExp(r'^[ \t]*[a-zA-Z_][\w.-]*(?=\s*[=:])'),
    const TextStyle(color: Color(0xFF56B6C2)),
  ),

  // Quoted strings
  SyntaxRule(
    RegExp(r'''"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*' '''),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // Boolean values
  SyntaxRule(
    RegExp(
      r'\b(true|false|yes|no|on|off|enabled|disabled)\b',
      caseSensitive: false,
    ),
    const TextStyle(color: Color(0xFFC678DD)),
  ),

  // Null values
  SyntaxRule(
    RegExp(r'\b(null|none|nil)\b', caseSensitive: false),
    const TextStyle(color: Color(0xFFE06C75)),
  ),

  // Numbers
  SyntaxRule(
    RegExp(r'\b\d+(?:\.\d+)?\b'),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // Assignment operators
  SyntaxRule(RegExp(r'[:=]'), const TextStyle(color: Color(0xFFABB2BF))),
];
