#!/usr/bin/env python3
"""Generate deterministic material-draw decision cases from local Syzygy tables."""

import argparse
import json
import random
from pathlib import Path

import chess
import chess.syzygy


TEMPLATES = (
    ("Q", "B"),
    ("Q", "N"),
    ("R", "B"),
    ("R", "N"),
)

PIECE_TYPES = {
    "Q": chess.QUEEN,
    "R": chess.ROOK,
    "B": chess.BISHOP,
    "N": chess.KNIGHT,
}


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--syzygy-path", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--seed", type=int, default=20260810)
    parser.add_argument("--force-draw-cases", type=int, default=8)
    parser.add_argument("--avoid-draw-cases", type=int, default=8)
    parser.add_argument("--attempts", type=int, default=200000)
    return parser.parse_args()


def random_board(rng):
    strong, weak = rng.choice(TEMPLATES)
    strong_color = rng.choice((chess.WHITE, chess.BLACK))
    weak_color = not strong_color
    squares = rng.sample(range(64), 4)

    board = chess.Board.empty()
    board.set_piece_at(squares[0], chess.Piece(chess.KING, chess.WHITE))
    board.set_piece_at(squares[1], chess.Piece(chess.KING, chess.BLACK))
    board.set_piece_at(squares[2], chess.Piece(PIECE_TYPES[strong], strong_color))
    board.set_piece_at(squares[3], chess.Piece(PIECE_TYPES[weak], weak_color))
    board.turn = rng.choice((chess.WHITE, chess.BLACK))
    board.castling_rights = chess.BB_EMPTY
    board.ep_square = None
    board.halfmove_clock = 0
    board.fullmove_number = 1
    return board


def probe_after_move(tablebase, board, move):
    child = board.copy(stack=False)
    child.push(move)
    if child.is_insufficient_material():
        return 0, 1

    try:
        outcome = -tablebase.probe_wdl(child)
    except KeyError:
        return None, 0

    for reply in child.legal_moves:
        grandchild = child.copy(stack=False)
        grandchild.push(reply)
        if grandchild.is_insufficient_material():
            return outcome, 2
    return outcome, 0


def classify(tablebase, board):
    try:
        current = tablebase.probe_wdl(board)
    except KeyError:
        return None

    moves = []
    for move in board.legal_moves:
        outcome, transition_plies = probe_after_move(tablebase, board, move)
        if outcome is None:
            return None
        moves.append(
            {
                "uci": move.uci(),
                "wdl": outcome,
                "material_draw_plies": transition_plies,
            }
        )

    if not moves:
        return None

    if current == 0:
        drawing = [move for move in moves if move["wdl"] == 0]
        losing = [move for move in moves if move["wdl"] < 0]
        if drawing and losing and all(move["material_draw_plies"] > 0 for move in drawing):
            return {
                "kind": "force_material_draw",
                "oracle_wdl": "draw",
                "expected_score": "cp -5",
                "acceptable_moves": sorted(move["uci"] for move in drawing),
                "forbidden_moves": sorted(move["uci"] for move in losing),
                "material_draw_plies": max(move["material_draw_plies"] for move in drawing),
            }

    if current == 2:
        winning = [move for move in moves if move["wdl"] == 2]
        traps = [move for move in moves if move["wdl"] == 0 and move["material_draw_plies"] > 0]
        if winning and traps:
            return {
                "kind": "avoid_material_draw",
                "oracle_wdl": "win",
                "minimum_score_cp": 1,
                "acceptable_moves": sorted(move["uci"] for move in winning),
                "forbidden_moves": sorted(move["uci"] for move in traps),
                "material_draw_plies": max(move["material_draw_plies"] for move in traps),
            }

    return None


def main():
    args = parse_args()
    rng = random.Random(args.seed)
    wanted = {
        "force_material_draw": args.force_draw_cases,
        "avoid_material_draw": args.avoid_draw_cases,
    }
    found = {kind: [] for kind in wanted}
    seen = set()

    with chess.syzygy.open_tablebase(args.syzygy_path) as tablebase:
        for _ in range(args.attempts):
            if all(len(found[kind]) >= wanted[kind] for kind in wanted):
                break
            board = random_board(rng)
            if not board.is_valid() or board.is_game_over(claim_draw=False):
                continue
            fen = board.fen(en_passant="fen")
            key = " ".join(fen.split()[:4])
            if key in seen:
                continue
            seen.add(key)

            case = classify(tablebase, board)
            if case is None or len(found[case["kind"]]) >= wanted[case["kind"]]:
                continue
            case["fen"] = fen
            found[case["kind"]].append(case)

    missing = {kind: wanted[kind] - len(found[kind]) for kind in wanted if len(found[kind]) < wanted[kind]}
    if missing:
        raise SystemExit(f"could not generate requested cases after {args.attempts} attempts: {missing}")

    cases = []
    for kind in ("force_material_draw", "avoid_material_draw"):
        for index, case in enumerate(sorted(found[kind], key=lambda item: item["fen"]), start=1):
            case["id"] = f"{kind}-{index:02d}"
            cases.append(case)

    suite = {
        "schema_version": 1,
        "source": {
            "oracle": "Syzygy WDL via python-chess 1.11.2",
            "seed": args.seed,
            "attempt_limit": args.attempts,
            "templates": [f"K{strong}vK{weak}" for strong, weak in TEMPLATES],
        },
        "cases": cases,
    }
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(suite, indent=2) + "\n", encoding="utf-8")
    print(f"generated {len(cases)} cases in {output}")


if __name__ == "__main__":
    main()
