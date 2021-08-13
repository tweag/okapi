let c = ['a'-'z']

rule lexy = parse
  | (c as c) eof
  {c}
