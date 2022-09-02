from time import *

# Prints First Frame 
def frame1():
  print("  ==================T==================")
  print("     .             |:|")
  print(":-. //           /\"\"\"\"\"\"-.")
  print("\': \'-._____..--\"\"(\"\"\"\"\"\"()`---.__")
  print(" /:   _..__   ''  \":\"\"\"\"\'[] |\"\"`\\\\\\\\")
  print(" \': :\'     `-.     _:._     \'\"\"\"\" :")
  print("  ::          \'--=:____:.___....-\"")
  print("                    O\"       O\" ")

# Prints Second Frame 
def frame2():
  print("  ______.........--=T=--.........______")
  print("     .             |:|")
  print(":-. //           /\"\"\"\"\"\"-.")
  print("\': \'-._____..--\"\"(\"\"\"\"\"\"()`---.__")
  print(" /:   _..__   ''  \":\"\"\"\"\'[] |\"\"`\\\\\\\\")
  print(" \': :\'     `-.     _:._     \'\"\"\"\" :")
  print("  ::          \'--=:____:.___....-\"")
  print("                    O\"       O\" ")

# Clear Console Window
def clearConsole():
  command = 'clear'
  if os.name in ('nt', 'dos'):
      command = 'cls'
  os.system(command)

# Prints Both Frame 1000 Times 
for x in range(1000):
  clearConsole()
  frame1()
  sleep(0.2)
  clearConsole()
  frame2()
  sleep(0.2)
