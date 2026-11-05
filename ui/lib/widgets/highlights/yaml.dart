import 'package:flutter/material.dart';
import 'package:nectar_apps_ui/widgets/highlights/syntax_rule.dart';

final yamlSyntaxRules = <SyntaxRule>[
  // Comments
  SyntaxRule(
    RegExp(r'(?<!\S)#[^\n]*'),
    const TextStyle(color: Color(0xFF7F848E), fontStyle: FontStyle.italic),
  ),

  // Document markers
  SyntaxRule(
    RegExp(r'^(?:---|\.\.\.)(?=\s|$)'),
    const TextStyle(color: Color(0xFFC678DD), fontWeight: FontWeight.bold),
  ),

  // Quoted strings
  SyntaxRule(
    RegExp(r'''"(?:[^"\\]|\\.)*"|'(?:[^']|'')*' '''),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // Block scalar indicators
  SyntaxRule(
    RegExp(r'(?<=:\s)[|>][+-]?\d*|^[ \t]*[|>][+-]?\d*'),
    const TextStyle(color: Color(0xFF61AFEF)),
  ),

  // Anchors and aliases
  SyntaxRule(
    RegExp(r'(?<!\w)[&*][a-zA-Z_][\w-]*'),
    const TextStyle(color: Color(0xFF61AFEF)),
  ),

  // Tags
  SyntaxRule(
    RegExp(r'![a-zA-Z_][\w:!-]*'),
    const TextStyle(color: Color(0xFFE5C07B)),
  ),

  // Mapping keys
  SyntaxRule(
    RegExp(
      r'^[ \t]*(?:[a-zA-Z_][\w.-]*|"[^"]+"|'
      '[^'
      ']+'
      ')(?=s*:)',
    ),
    const TextStyle(color: Color(0xFF56B6C2)),
  ),

  // Booleans
  SyntaxRule(
    RegExp(r'\b(?:true|false|yes|no|on|off)\b', caseSensitive: false),
    const TextStyle(color: Color(0xFFC678DD)),
  ),

  // Null values
  SyntaxRule(
    RegExp(r'\b(?:null|~)\b', caseSensitive: false),
    const TextStyle(color: Color(0xFFE06C75)),
  ),

  // Numbers
  SyntaxRule(
    RegExp(r'(?<![\w.-])[+-]?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b'),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // Mapping and sequence punctuation
  SyntaxRule(RegExp(r'[:\[\]{},]'), const TextStyle(color: Color(0xFFABB2BF))),
];
