package parms

// Parms is an array with all evaluation values
var Parms = [...]int{
	// --------------- pawn parms -----------------------
	6,     // 6,   //6,   //6,  //5,  		// unstoppable passed
	10,     // 10,   //10,   //11, //20, 		// MG passed
	31,     // 31,   //31,   //29, //25, 		// EG passed
	12,     // 12,   //12,   //13, //10, 		// EG passed dist my king
	26,     // 26,   //26,   //24, //20, 		// EG passed dist opp king
	  //ix=5 (next)
	11,     // 10,   //11, //4,  		// weight for # pawn moves to empty
	10,     // 10,   //6,  //2,  		// weight for # pawns
	53,     // 53,   //53, //50, 		// parm pawn capt
	2,     // 2,    //2,  //2,  		// parm pawn capt
	16,     // 16,   //16, //10, 		// parm pawn capt
	  //ix=10
	7,     // 7,   //5, //4,   	// weight passed
	10,     // 10,  //6, //2,   	// weght adj passed
	7,     // 7,   //8, //8,   	// dist weight
	3,     // 3,   //3, //3,   	// dist weight parm 1
	7,     // 7,   //7, //7.0, 	// dist weight parm 2
	  //ix=15 (next)
	256,     // 256, //256, //256.0, 	// dist weight parm 3
	2,     // 2,   //2,   //2,     	// passed weight rk 3
	7,     // 7,   //6,   //6,     	// passed weight rk 4
	13,     // 13,  //11,  //12,    	// passed weight rk 5
	25,     // 25,  //23,  //20,    	// passed weight rk 6
	  //ix=20 (next)
	5,     // 5,   //6,   //4, 	  	// passed weight mul  4
	81,     // 81,  //89,  // MG pawn value    89
	101,     // 101, //95,  // EG pawn value
	300,     // 300, //324, // MG knight value  	// --------------- piece parms -----------------------
	307,     // 307, //326, // EG knight value
	  //ix=25 (next)
	320,     // 320,   //324, // MG bishop value
	322,     // 322,   //326, // EG bisop value
	467,     // 467,   //460, // MG rook value
	536,     // 536,   //540, // EG rook value
	1048,     // 975,   //975, // MG queen value
	  //ix=30 (next)
	1027,     // 975, //975, // EG queen value
	18,     // 18,  //20,  // mobilityscore weight 1
	9,     // 9,   //8,   // mobilityscore weight 2
	5,     // 5,   //4,   // attackMG weight 1
	1,     // 1,   //2,   // attackMG weight 2
	  //ix=35 (next)
	1,     // 1,   // 4, // attackWeight knight
	3,     // 3,   //4, // attackWeight bishop
	5,     // 5,   //2, // attackWeight rook
	2,     // 2,   //1, // attackWeight queen
	1,     // 1,   //4, // attackWeight king
	   //ix=40 (next)
	2,     // 2,   //1, // attackedWeight knight
	2,     // 2,   //1, // attackedWeight bishop
	2,     // 2,   //2, // attackedWeight rook
	7,     // 6,   //4, // attackedWeight queen
	3,     // 4,   //8, // attackedWeight king
	  //ix=45 (next)
	1,     // 1,   // 2, // captureScore weigth 1
	3,     // 3,   //4, // captureScore weigth 2
	2,     // 2,   //1, // powervalue knight
	1,     // 1,   //1, // powervalue bishop
	3,     // 3,   //2, // powervalue rook
	  //ix=50 (next)
	5,     // 5,   //4, // powervalue queen
	1,     // 1,   //2, // checkNumber weight
	6,     // 6,   //5, // evalOutpost weight
	5,     // 4,   //2, // evalOutpost weight 2
	5,     // 5,   //1, // evalOutpost weight 3
	  //ix=55 (next)
	2,     // 2,   //1,  // evalOutpost weight 4
	2,     // 2,   //2,  // evalOutpost weight 5
	1,     // 1,   //10, // not guarded minor
	4,     // 4,   //10, // shielded minor
	10,     // 10,  //10, // R blocked by minor
	  //ix=60 (next)
	9,     // 9,   // 5,  // R blocked by minor
	6,     // 17,  //10, // R blocked by minor
	10,     // 10,  //10, // R on open file with K
	1,     // 1,   //1,  // R on open file with K weight
	4,     // 4,   //2,  // R on open file with K weight
	  //ix=65 (next)
	1,     // 1,   //2,  // R on open file with K weight
	16,     // 16,  //10, // R on 7th MG
	26,     // 26,  //20, // R on 7th EG
	3,     // 4,   //20, // K ditance to A and H  EG
	14,     // 14,  //30, // bishop pair bonus MG
	  //ix=70 (next)
	43,     // 43,   //50,  // bishop pair bonus EG
	200,     // 200,  //200, // shelter score  (from 2 to 200 I scaled it up by 100)
	34,     // 34,   //30,  // KingPower in Kingscore
	32,     // 32,   //32,  // KingScore parm
	5,     // 5,    //5,   // KingScore bias
	  //ix=75 (next)
	8,     // 8,   //8,  // KingScore weight  8
	1,     // 1,   //20, // Fiancetto bonus B2 or B7
	17,     // 17,   //20, // Fiancetto bonus G2 or G7
}

/*
6,     // 
10,     // 
31,     // 
12,     // 
26,     // 
 
11,     // 
10,     // 
53,     // 
2,     // 
16,     // 
 
7,     // 
10,     // 
7,     // 
3,     // 
7,     // 
 
256,     // 
2,     // 
7,     // 
13,     // 
25,     // 
 
5,     // 
81,     // 
101,     // 
300,     // 
307,     // 
 
320,     // 
322,     // 
467,     // 
536,     // 
1048,     // 
 
1027,     // 
18,     // 
9,     // 
5,     // 
1,     // 
 
1,     // 
3,     // 
5,     // 
2,     // 
1,     // 
 
2,     // 
2,     // 
2,     // 
7,     // 
3,     // 
 
1,     // 
3,     // 
2,     // 
1,     // 
3,     // 
 
5,     // 
1,     // 
6,     // 
5,     // 
5,     // 
 
2,     // 
2,     // 
1,     // 
4,     // 
10,     // 
 
9,     // 
6,     // 
10,     // 
1,     // 
4,     // 
 
1,     // 
16,     // 
26,     // 
3,     // 
14,     // 
 
43,     // 
200,     // 
34,     // 
32,     // 
5,     // 
 
8,     // 
1,     // 
17,     // 
*/

//Nparms is the number of evaluation parameters. It is used by tuner and should be len(Parma)
const Nparms = 78
