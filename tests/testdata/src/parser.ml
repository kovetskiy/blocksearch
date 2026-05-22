(* OCaml: searching the function name returns its whole let binding. *)

let compute_total subtotal =
  let tax = subtotal /. 5.0 in
  subtotal +. tax

let () =
  print_float (compute_total 100.0)
