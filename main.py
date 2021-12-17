from time import *

def frame1():
  print("  ==================T==================")
  print("     .             |:|")
  print(":-. //           /\"\"\"\"\"\"-.")
  print("\': \'-._____..--\"\"(\"\"\"\"\"\"()`---.__")
  print(" /:   _..__   ''  \":\"\"\"\"\'[] |\"\"`\\\\\\\\")
  print(" \': :\'     `-.     _:._     \'\"\"\"\" :")
  print("  ::          \'--=:____:.___....-\"")
  print("                    O\"       O\" ")

def frame2():
  print("  ______.........--=T=--.........______")
  print("     .             |:|")
  print(":-. //           /\"\"\"\"\"\"-.")
  print("\': \'-._____..--\"\"(\"\"\"\"\"\"()`---.__")
  print(" /:   _..__   ''  \":\"\"\"\"\'[] |\"\"`\\\\\\\\")
  print(" \': :\'     `-.     _:._     \'\"\"\"\" :")
  print("  ::          \'--=:____:.___....-\"")
  print("                    O\"       O\" ")

def clearConsole():
    command = 'clear'
    if os.name in ('nt', 'dos'):  # If Machine is running on Windows, use cls
        command = 'cls'
    os.system(command)

for x in range(1000):
  clearConsole()
  frame1()
  sleep(0.2)
  clearConsole()
  frame2()
  sleep(0.2)
