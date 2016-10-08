package util

import (
	"fmt"
	"math/rand"
	"runtime"
)

// ASSERT is asserting message f cond is false
func ASSERT(cond bool, msg ...interface{}) {

	//	if !cond {
	//		if len(msg) > 0 {
	//			panic("assert failed. ")
	//		} else {
	//			panic("assert failed. ")
	//		}
	//	}
}

// Profiling = true means that we are running a profiling test
var Profiling = false

// TellGUI prints a line to stdout (to the GUI)
func TellGUI(line string) {
	fmt.Println(line)
}

var r1 = (*rand.Rand)(rand.New(rand.NewSource(42)))

// RandInt returns a random integer number
func RandInt(n int) int {
	//assert(n > 0);
	return r1.Intn(n)
}

// Iabs returns the absolute value of an int
func Iabs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// Imax returns maximum value of two ints
func Imax(a, b int) int {
	if a >= b {
		return a
	}
	return b
}

// Imin returns minimum value of two ints
func Imin(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

// EndianCheck is a test function to determine if the processor is using Big or Low Endian
func EndianCheck() {

	var ourOrderIsBE bool

	// case will need maintenace with more BE archs coming

	switch runtime.GOARCH {
	case "mips64", "ppc64":
		ourOrderIsBE = true
	}

	if ourOrderIsBE {
		TellGUI("info string BigEndian")
	} else {
		TellGUI("info string LowEndian")
	}
}

/*


   public:

      Timer() {
         reset();
      }

      void reset() {
         p_elapsed = 0;
         p_running = false;
      }

      void start() {
         p_start = now();
         p_running = true;
      }

      void stop() {
         p_elapsed += time();
         p_running = false;
      }

      int elapsed() const {
         int time = p_elapsed;
         if (p_running) time += this->time();
         return time;
      }

   };

   class Lockable {

   protected: // HACK for Waitable::wait()

      mutable std::mutex p_mutex;

   public:

      void lock   () const { p_mutex.lock(); }
      void unlock () const { p_mutex.unlock(); }
   };

   class Waitable : public Lockable {

   private:

      std::condition_variable_any p_cond;

   public:

      void wait   () { p_cond.wait(p_mutex); } // HACK: direct access
      void signal () { p_cond.notify_one(); }
   };

   int round(double x) {
      return int(std::floor(x + 0.5));
   }

   int div(int a, int b) {

      assert(b > 0);

      int div = a / b;
      if (a < 0 && a != b * div) div--; // fix buggy C semantics

      return div;
   }

   int sqrt(int n) {
      return int(std::sqrt(double(n)));
   }

   bool is_square(int n) {
      int i = sqrt(n);
      return i * i == n;
   }





   int string_find(const std::string & s, char c) {
      return int(s.find(c));
   }

   bool string_case_equal(const std::string & s0, const std::string & s1) {

      if (s0.size() != s1.size()) return false;

      for (int i = 0; i < int(s0.size()); i++) {
         if (std::tolower(s0[i]) != std::tolower(s1[i])) return false;
      }

      return true;
   }

   bool to_bool(const std::string & s) {
      if (string_case_equal(s, "true")) {
         return true;
      } else if (string_case_equal(s, "false")) {
         return false;
      } else {
         std::cerr << "not a boolean: \"" << s << "\"" << std::endl;
         std::exit(EXIT_FAILURE);
      }
   }

   int64 to_int(const std::string & s) {
      std::stringstream ss(s);
      int64 n;
      ss >> n;
      return n;
   }

   std::string to_string(int n) {
      std::stringstream ss;
      ss << n;
      return ss.str();
   }

   std::string to_string(double x) {
      std::stringstream ss;
      ss << x;
      return ss.str();
   }

   void log(const std::string & s) {
      std::ofstream log_file("log.txt", std::ios_base::app);
      log_file << s << std::endl;
   }
}
*/
